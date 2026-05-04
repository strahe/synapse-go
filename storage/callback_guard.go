package storage

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

const uploadCallbackPanicMessage = "storage upload callback panic ignored"

type uploadCallbackGuard struct {
	logger *slog.Logger

	mu     sync.Mutex
	warned map[string]struct{}
}

func newUploadCallbackGuard(logger *slog.Logger) *uploadCallbackGuard {
	return &uploadCallbackGuard{logger: logger}
}

func (g *uploadCallbackGuard) safeInvoke(name string, fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			g.warnOnce(name, recovered)
		}
	}()
	fn()
}

func (g *uploadCallbackGuard) warnOnce(name string, recovered any) {
	if g.logger == nil {
		return
	}
	g.mu.Lock()
	if g.warned == nil {
		g.warned = make(map[string]struct{})
	}
	if _, ok := g.warned[name]; ok {
		g.mu.Unlock()
		return
	}
	g.warned[name] = struct{}{}
	g.mu.Unlock()

	g.logger.Warn(uploadCallbackPanicMessage, "callback", name, "panic", fmt.Sprint(recovered))
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
