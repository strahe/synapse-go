package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

// errUploadCallbackPanicked cancels the rest of an upload after a callback
// panics.
var errUploadCallbackPanicked = errors.New("storage: upload callback panicked")

// uploadCallbackGuard runs upload callbacks, which may execute on internal
// goroutines where a panic cannot reach the caller. The first panic cancels
// the upload and suppresses later callbacks; rethrow re-raises it with its
// original value on the caller's goroutine after the upload has unwound.
type uploadCallbackGuard struct {
	op     string
	logger *slog.Logger
	cancel context.CancelCauseFunc

	mu   sync.Mutex
	done bool
	// panicValue is the first recovered panic value; recover never returns
	// nil for a panic.
	panicValue any
}

// newUploadCallbackGuard returns a context that the guard cancels when a
// callback panics. Callers must defer rethrow.
func newUploadCallbackGuard(ctx context.Context, op string, logger *slog.Logger) (context.Context, *uploadCallbackGuard) {
	ctx, cancel := context.WithCancelCause(ctx)
	return ctx, &uploadCallbackGuard{op: op, logger: logger, cancel: cancel}
}

func (g *uploadCallbackGuard) safeInvoke(name string, fn func()) {
	if fn == nil {
		return
	}
	g.mu.Lock()
	skip := g.done || g.panicValue != nil
	g.mu.Unlock()
	if skip {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.record(name, recovered, debug.Stack())
		}
	}()
	fn()
}

func (g *uploadCallbackGuard) record(name string, value any, stack []byte) {
	g.mu.Lock()
	first := g.panicValue == nil
	if first {
		g.panicValue = value
	}
	g.mu.Unlock()
	if !first {
		return
	}
	g.cancel(errUploadCallbackPanicked)
	if g.logger != nil {
		g.logger.Error(g.op+": callback panicked",
			"callback", name, "panic", fmt.Sprint(value), "stack", string(stack))
	}
}

// rethrow stops later callbacks and re-raises the first recorded panic.
func (g *uploadCallbackGuard) rethrow() {
	g.mu.Lock()
	g.done = true
	panicValue := g.panicValue
	g.mu.Unlock()
	g.cancel(nil)
	if panicValue != nil {
		panic(panicValue)
	}
}

func (g *uploadCallbackGuard) wrapUploadOptions(opts *UploadOptions) *UploadOptions {
	if opts == nil {
		return nil
	}
	wrapped := *opts
	if opts.OnProgress != nil {
		onProgress := opts.OnProgress
		wrapped.OnProgress = func(bytesUploaded int64) {
			g.safeInvoke("OnProgress", func() {
				onProgress(bytesUploaded)
			})
		}
	}
	if opts.OnStored != nil {
		onStored := opts.OnStored
		wrapped.OnStored = func(providerID types.BigInt, pieceCID cid.Cid) {
			g.safeInvoke("OnStored", func() {
				onStored(providerID, pieceCID)
			})
		}
	}
	if opts.OnPiecesAdded != nil {
		onPiecesAdded := opts.OnPiecesAdded
		wrapped.OnPiecesAdded = func(txHash string, providerID types.BigInt, pieces []SubmittedPiece) {
			g.safeInvoke("OnPiecesAdded", func() {
				onPiecesAdded(txHash, providerID, pieces)
			})
		}
	}
	if opts.OnPiecesConfirmed != nil {
		onPiecesConfirmed := opts.OnPiecesConfirmed
		wrapped.OnPiecesConfirmed = func(dataSetID, providerID types.BigInt, pieces []ConfirmedPiece) {
			g.safeInvoke("OnPiecesConfirmed", func() {
				onPiecesConfirmed(dataSetID, providerID, pieces)
			})
		}
	}
	if opts.OnCopyComplete != nil {
		onCopyComplete := opts.OnCopyComplete
		wrapped.OnCopyComplete = func(providerID types.BigInt, pieceCID cid.Cid) {
			g.safeInvoke("OnCopyComplete", func() {
				onCopyComplete(providerID, pieceCID)
			})
		}
	}
	if opts.OnCopyFailed != nil {
		onCopyFailed := opts.OnCopyFailed
		wrapped.OnCopyFailed = func(providerID types.BigInt, pieceCID cid.Cid, err error) {
			g.safeInvoke("OnCopyFailed", func() {
				onCopyFailed(providerID, pieceCID, err)
			})
		}
	}
	if opts.OnPullProgress != nil {
		onPullProgress := opts.OnPullProgress
		wrapped.OnPullProgress = func(providerID types.BigInt, pieceCID cid.Cid, status PullStatus) {
			g.safeInvoke("OnPullProgress", func() {
				onPullProgress(providerID, pieceCID, status)
			})
		}
	}
	return &wrapped
}
