package payments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"syscall"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/internal/txutil"
)

// Each summary owns its binding and block; the Service's shared backend stays
// unchanged when concurrent summaries take different snapshots.
type accountSummaryReader struct {
	backend bind.ContractCaller
	block   *big.Int
	filPay  *filpay.FilPayCaller
}

func (s *Service) newAccountSummaryReader(ctx context.Context) (*accountSummaryReader, error) {
	block, err := retryAccountRead(ctx, s.backend.BlockNumber)
	if err != nil {
		return nil, fmt.Errorf("block number: %w", err)
	}
	r := &accountSummaryReader{backend: s.backend, block: new(big.Int).SetUint64(block)}
	r.filPay, err = filpay.NewFilPayCaller(s.filPayAddr, r)
	if err != nil {
		return nil, fmt.Errorf("bind summary caller: %w", err)
	}
	return r, nil
}

func (r *accountSummaryReader) CallContract(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return retryAccountRead(ctx, func(ctx context.Context) ([]byte, error) {
		return r.backend.CallContract(ctx, msg, copyBig(r.block))
	})
}

func (r *accountSummaryReader) CodeAt(ctx context.Context, address common.Address, _ *big.Int) ([]byte, error) {
	return retryAccountRead(ctx, func(ctx context.Context) ([]byte, error) {
		return r.backend.CodeAt(ctx, address, copyBig(r.block))
	})
}

func retryAccountRead[T any](ctx context.Context, read func(context.Context) (T, error)) (T, error) {
	return retry.Do(ctx, read,
		retry.WithMaxRetries(2),
		retry.WithInitialDelay(200*time.Millisecond),
		retry.WithMultiplier(2),
		retry.WithMaxDelay(2*time.Second),
		retry.WithRetryIf(isRetryableAccountRead),
	)
}

func isRetryableAccountRead(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if httpErr, ok := errors.AsType[rpc.HTTPError](err); ok {
		return retryableAccountHTTPStatus(httpErr.StatusCode)
	}
	if httpErr, ok := errors.AsType[*rpc.HTTPError](err); ok {
		return retryableAccountHTTPStatus(httpErr.StatusCode)
	}
	if rpcErr, ok := errors.AsType[rpc.Error](err); ok {
		switch rpcErr.ErrorCode() {
		case 3, -32700, -32600, -32601, -32602:
			return false
		}
		message := strings.ToLower(rpcErr.Error())
		if strings.Contains(message, "revert") || strings.Contains(message, "invalid argument") || strings.Contains(message, "invalid param") {
			return false
		}
		if strings.Contains(message, "rate limit") || strings.Contains(message, "too many requests") {
			return true
		}
		switch rpcErr.ErrorCode() {
		case -32000, -32002, -32603:
			return strings.Contains(message, "timeout") || strings.Contains(message, "timed out")
		default:
			return false
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	if netErr, ok := errors.AsType[net.Error](err); ok {
		return netErr.Timeout()
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "revert") || strings.Contains(message, "invalid argument") || strings.Contains(message, "invalid param") {
		return false
	}
	return txutil.IsRetryableRPCError(err)
}

func retryableAccountHTTPStatus(status int) bool {
	return status == 429 || status == 502 || status == 503 || status == 504
}
