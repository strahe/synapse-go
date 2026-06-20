package pdp

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

// TerminateServiceRequest requests provider-relayed FWSS service
// termination.
type TerminateServiceRequest struct {
	DataSetID types.BigInt
	ExtraData []byte
}

// TerminateServiceResult contains the provider status URL to poll.
type TerminateServiceResult struct {
	StatusURL string
}

// TerminateServiceStatus is the provider's termination status.
type TerminateServiceStatus struct {
	TerminationTxHash       *common.Hash
	FWSSTerminated          bool
	ServiceTerminationEpoch types.Epoch
}

type terminateServiceWire struct {
	ExtraData string `json:"extraData"`
}

type terminateConflictWire struct {
	Code                    int          `json:"code"`
	Message                 string       `json:"message"`
	ServiceTerminationEpoch *json.Number `json:"serviceTerminationEpoch"`
}

type terminateStatusWire struct {
	TerminationTxHash       string       `json:"terminationTxHash"`
	FWSSTerminated          *bool        `json:"fwssTerminated"`
	ServiceTerminationEpoch *json.Number `json:"serviceTerminationEpoch"`
}

// TerminateServiceStatusURL returns the provider status URL for a data set.
func (c *Client) TerminateServiceStatusURL(dataSetID types.BigInt) (string, error) {
	if dataSetID.IsZero() {
		return "", fmt.Errorf("pdp.TerminateServiceStatusURL: zero dataSetID")
	}
	u, err := c.resolve(fmt.Sprintf("pdp/data-sets/%s/terminate", dataSetID.String()))
	if err != nil {
		return "", fmt.Errorf("pdp.TerminateServiceStatusURL: %w", err)
	}
	return u.String(), nil
}

// TerminateService requests provider-relayed service termination.
func (c *Client) TerminateService(ctx context.Context, req TerminateServiceRequest) (*TerminateServiceResult, error) {
	if req.DataSetID.IsZero() {
		return nil, fmt.Errorf("pdp.TerminateService: zero dataSetID")
	}
	if len(req.ExtraData) == 0 {
		return nil, fmt.Errorf("pdp.TerminateService: empty extraData")
	}
	path := fmt.Sprintf("pdp/data-sets/%s/terminate", req.DataSetID.String())
	wire := terminateServiceWire{ExtraData: "0x" + hex.EncodeToString(req.ExtraData)}
	_, body, err := c.postJSONRetry429(ctx, path, wire, http.StatusAccepted)
	if err != nil {
		return nil, c.mapTerminateServiceError(err, body)
	}
	statusURL, err := c.TerminateServiceStatusURL(req.DataSetID)
	if err != nil {
		return nil, err
	}
	return &TerminateServiceResult{StatusURL: statusURL}, nil
}

func (c *Client) mapTerminateServiceError(err error, body []byte) error {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok {
		return err
	}
	switch httpErr.StatusCode {
	case http.StatusConflict:
		var conflict terminateConflictWire
		if jsonErr := json.Unmarshal(body, &conflict); jsonErr != nil {
			return fmt.Errorf("pdp.TerminateService: decode conflict: %w", jsonErr)
		}
		switch conflict.Code {
		case 0:
			epoch, convErr := epochFromJSONNumber(conflict.ServiceTerminationEpoch)
			if convErr != nil {
				return fmt.Errorf("pdp.TerminateService: decode terminated epoch: %w", convErr)
			}
			return &ServiceAlreadyTerminatedError{ServiceTerminationEpoch: epoch, Message: conflict.Message}
		case 1:
			return &TerminateServicePendingError{Message: conflict.Message}
		default:
			return fmt.Errorf("pdp.TerminateService: %w", err)
		}
	case http.StatusServiceUnavailable:
		return &TerminateServiceNotSupportedError{Body: httpErr.Body}
	default:
		return fmt.Errorf("pdp.TerminateService: %w", err)
	}
}

// GetTerminateServiceStatus reads the provider termination status once.
func (c *Client) GetTerminateServiceStatus(ctx context.Context, dataSetID types.BigInt) (*TerminateServiceStatus, error) {
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("pdp.GetTerminateServiceStatus: zero dataSetID")
	}
	var wire terminateStatusWire
	if err := c.getJSON(ctx, fmt.Sprintf("pdp/data-sets/%s/terminate", dataSetID.String()), &wire); err != nil {
		return nil, err
	}
	return parseTerminateStatus(wire)
}

func parseTerminateStatus(wire terminateStatusWire) (*TerminateServiceStatus, error) {
	var hash *common.Hash
	if wire.TerminationTxHash != "" {
		if !common.IsHexHash(wire.TerminationTxHash) {
			return nil, fmt.Errorf("pdp.GetTerminateServiceStatus: invalid terminationTxHash %q", wire.TerminationTxHash)
		}
		h := common.HexToHash(wire.TerminationTxHash)
		hash = &h
	}
	if wire.FWSSTerminated == nil || !*wire.FWSSTerminated {
		return &TerminateServiceStatus{TerminationTxHash: hash}, nil
	}
	epoch, err := epochFromJSONNumber(wire.ServiceTerminationEpoch)
	if err != nil {
		return nil, fmt.Errorf("pdp.GetTerminateServiceStatus: decode serviceTerminationEpoch: %w", err)
	}
	return &TerminateServiceStatus{
		TerminationTxHash:       hash,
		FWSSTerminated:          true,
		ServiceTerminationEpoch: epoch,
	}, nil
}

// WaitForTerminateService polls termination status until FWSS termination is
// confirmed.
func (c *Client) WaitForTerminateService(
	ctx context.Context,
	dataSetID types.BigInt,
	pollInterval time.Duration,
	onHash func(common.Hash),
) (*TerminateServiceStatus, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}
	var seenHash *common.Hash
	for {
		status, err := c.GetTerminateServiceStatus(ctx, dataSetID)
		if err != nil {
			httpErr, ok := errors.AsType[*HTTPError](err)
			if ok && httpErr.StatusCode == http.StatusNotFound {
				if seenHash == nil {
					return nil, &WaitForTerminateServiceNotFoundError{}
				}
				return nil, &WaitForTerminateServiceRejectedError{TxHash: *seenHash}
			}
			return nil, fmt.Errorf("pdp.WaitForTerminateService: %w", err)
		}
		if status.TerminationTxHash != nil && seenHash == nil {
			h := *status.TerminationTxHash
			seenHash = &h
			if onHash != nil {
				onHash(h)
			}
		}
		if status.FWSSTerminated {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (c *Client) postJSONRetry429(ctx context.Context, path string, payload any, expect ...int) (*http.Response, []byte, error) {
	u, err := c.resolve(path)
	if err != nil {
		return nil, nil, fmt.Errorf("pdp: resolve %s: %w", path, err)
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("pdp: marshal %s: %w", path, err)
	}
	maxRetries := c.maxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
		if err != nil {
			return nil, nil, fmt.Errorf("pdp: build POST %s: %w", path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, body, err := c.do(req, expect...)
		if err == nil {
			return resp, body, nil
		}
		httpErr, retry := errors.AsType[*HTTPError](err)
		if !retry || httpErr.StatusCode != http.StatusTooManyRequests || attempt == maxRetries {
			return resp, body, err
		}
		delayFn := c.retryDelayFn
		if delayFn == nil {
			delayFn = httpRetryDelay
		}
		delay := delayFn(err, attempt)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, nil, fmt.Errorf("pdp: postJSONRetry429: retry loop exited without result")
}

func epochFromJSONNumber(v *json.Number) (types.Epoch, error) {
	if v == nil {
		return 0, fmt.Errorf("missing epoch")
	}
	i, ok := new(big.Int).SetString(v.String(), 10)
	if !ok || i.Sign() < 0 || !i.IsUint64() {
		return 0, fmt.Errorf("invalid epoch %q", v.String())
	}
	return types.Epoch(i.Uint64()), nil
}
