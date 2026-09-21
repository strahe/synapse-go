package pdp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/redact"
)

type transactionStatus uint8

const (
	transactionPending transactionStatus = iota
	transactionConfirmed
	transactionRejected
)

func parseRequiredStatusHash(op, field, value string) (common.Hash, error) {
	if !common.IsHexHash(value) {
		return common.Hash{}, fmt.Errorf("%s: %w: invalid %s", op, ErrInvalidStatus, field)
	}
	hash := common.HexToHash(value)
	if hash == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("%s: %w: zero %s", op, ErrInvalidStatus, field)
	}
	return hash, nil
}

func parseOptionalStatusHash(op, field, value string) (common.Hash, error) {
	if value == "" {
		return common.Hash{}, nil
	}
	return parseRequiredStatusHash(op, field, value)
}

func (c *Client) statusURLTransactionHash(op, statusURL string) (common.Hash, error) {
	if statusURL == "" {
		return common.Hash{}, fmt.Errorf("%s: %w: empty status URL", op, ErrInvalidStatusURL)
	}
	u, err := url.Parse(statusURL)
	if err != nil {
		return common.Hash{}, fmt.Errorf("%s: %w", op, ErrInvalidStatusURL)
	}
	if err := c.validateStatusURL(u); err != nil {
		return common.Hash{}, fmt.Errorf("%s: %w", op, err)
	}
	hashHex := lastPathSegment(statusURL)
	if hashHex == "" {
		return common.Hash{}, fmt.Errorf("%s: %w: missing transaction hash", op, ErrInvalidStatusURL)
	}
	if !strings.HasPrefix(hashHex, "0x") {
		hashHex = "0x" + hashHex
	}
	if !common.IsHexHash(hashHex) {
		return common.Hash{}, fmt.Errorf("%s: %w: invalid transaction hash", op, ErrInvalidStatusURL)
	}
	hash := common.HexToHash(hashHex)
	if hash == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("%s: %w: zero transaction hash", op, ErrInvalidStatusURL)
	}
	return hash, nil
}

func invalidStatusf(op, format string, args ...any) error {
	return fmt.Errorf("%s: %w: %s", op, ErrInvalidStatus, fmt.Sprintf(format, args...))
}

func classifyCreateDataSetStatus(op string, status *CreateDataSetStatus) (transactionStatus, error) {
	if status == nil {
		return transactionPending, invalidStatusf(op, "nil response")
	}
	if status.DataSetCreated {
		if status.DataSetID == nil || status.DataSetID.IsZero() {
			return transactionPending, invalidStatusf(op, "dataSetCreated without dataSetId")
		}
	} else if status.DataSetID != nil {
		return transactionPending, invalidStatusf(op, "dataSetId without dataSetCreated")
	}

	switch status.TxStatus {
	case "pending":
		if status.OK != nil || status.DataSetCreated {
			return transactionPending, invalidStatusf(op, "pending response contains terminal fields")
		}
		return transactionPending, nil
	case "reorged":
		// A reorged transaction is no longer canonical. Stage-local success
		// fields may still describe its pre-reorg result.
		return transactionRejected, nil
	case "failed", "rejected":
		if status.DataSetCreated || (status.OK != nil && *status.OK) {
			return transactionPending, invalidStatusf(op, "%s response contains successful fields", status.TxStatus)
		}
		return transactionRejected, nil
	case "confirmed":
		if status.OK == nil {
			if status.DataSetCreated {
				return transactionPending, invalidStatusf(op, "dataSetCreated without ok")
			}
			return transactionPending, nil
		}
		if !*status.OK {
			if status.DataSetCreated {
				return transactionPending, invalidStatusf(op, "rejected response reports dataSetCreated")
			}
			return transactionRejected, nil
		}
		if status.DataSetCreated {
			return transactionConfirmed, nil
		}
		return transactionPending, nil
	default:
		return transactionPending, invalidStatusf(op, "unknown txStatus %q", status.TxStatus)
	}
}

func classifyAddPiecesStatus(op string, status *AddPiecesStatus) (transactionStatus, error) {
	if status == nil {
		return transactionPending, invalidStatusf(op, "nil response")
	}
	if status.DataSetID.IsZero() {
		return transactionPending, invalidStatusf(op, "zero dataSetId")
	}
	if status.PieceCount < 0 {
		return transactionPending, invalidStatusf(op, "negative pieceCount")
	}
	if status.TxStatus == "reorged" {
		// The transaction is no longer canonical. A provider can retain stage-local
		// success fields without retaining the confirmed piece IDs.
		return transactionRejected, nil
	}
	if status.PiecesAdded && (status.AddMessageOK == nil || !*status.AddMessageOK) {
		return transactionPending, invalidStatusf(op, "piecesAdded without successful add message")
	}
	if len(status.ConfirmedPieceIDs) > 0 && !status.PiecesAdded {
		return transactionPending, invalidStatusf(op, "confirmedPieceIds without piecesAdded")
	}
	if status.PiecesAdded {
		if status.PieceCount == 0 {
			return transactionPending, invalidStatusf(op, "piecesAdded with zero pieceCount")
		}
		if status.PieceCount != len(status.ConfirmedPieceIDs) {
			return transactionPending, invalidStatusf(
				op,
				"pieceCount %d does not match confirmedPieceIds count %d",
				status.PieceCount,
				len(status.ConfirmedPieceIDs),
			)
		}
	}

	switch status.TxStatus {
	case "pending":
		if status.AddMessageOK != nil || status.PiecesAdded || len(status.ConfirmedPieceIDs) > 0 {
			return transactionPending, invalidStatusf(op, "pending response contains terminal fields")
		}
		return transactionPending, nil
	case "failed", "rejected":
		if (status.AddMessageOK != nil && *status.AddMessageOK) || status.PiecesAdded || len(status.ConfirmedPieceIDs) > 0 {
			return transactionPending, invalidStatusf(op, "%s response contains successful fields", status.TxStatus)
		}
		return transactionRejected, nil
	case "confirmed":
		if status.AddMessageOK == nil {
			if status.PiecesAdded || len(status.ConfirmedPieceIDs) > 0 {
				return transactionPending, invalidStatusf(op, "piecesAdded without addMessageOk")
			}
			return transactionPending, nil
		}
		if !*status.AddMessageOK {
			return transactionRejected, nil
		}
		if status.PiecesAdded {
			return transactionConfirmed, nil
		}
		return transactionPending, nil
	default:
		return transactionPending, invalidStatusf(op, "unknown txStatus %q", status.TxStatus)
	}
}

func (c *Client) resolveStatusURL(ref string) (string, error) {
	u, err := c.resolve(ref)
	if err != nil {
		return "", fmt.Errorf("%w: invalid status URL", ErrInvalidStatusURL)
	}
	if err := c.validateStatusURL(u); err != nil {
		return "", err
	}
	return u.String(), nil
}

func (c *Client) getStatusBody(ctx context.Context, op, statusURL string, expectStatuses ...int) ([]byte, error) {
	u, err := url.Parse(statusURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, ErrInvalidStatusURL)
	}
	if err := c.validateStatusURL(u); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	client := c.statusHTTPClient()
	_, body, err := c.doRetryableWithClient(ctx, client, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, expectStatuses...)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *Client) validateStatusURL(u *url.URL) error {
	if u == nil || !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%w: %w: %s", ErrInvalidStatusURL, ErrStatusURLOrigin, redact.URL(u))
	}
	if !sameOrigin(c.baseURL, u) {
		return fmt.Errorf("%w: %w: %s", ErrInvalidStatusURL, ErrStatusURLOrigin, redact.URL(u))
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (c *Client) statusHTTPClient() *http.Client {
	client := *c.httpClient
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := c.validateStatusURL(req.URL); err != nil {
			return err
		}
		if previous != nil {
			return previous(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}
