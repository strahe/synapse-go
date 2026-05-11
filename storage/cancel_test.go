package storage

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// ctxAwareResolver honors ctx.Done(), letting us drive Service.Upload and
// Service.CreateContexts down a cancellation path
// without setting up real provider plumbing.
type ctxAwareResolver struct{}

func (ctxAwareResolver) ResolveUploadContexts(ctx context.Context, _ *UploadOptions) ([]UploadContext, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	<-ctx.Done()
	return nil, false, ctx.Err()
}

func (ctxAwareResolver) SelectReplacement(ctx context.Context, _ map[string]types.BigInt, _ *UploadOptions) (UploadContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (ctxAwareResolver) ResolveContexts(ctx context.Context, _ *UploadOptions) ([]*Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestServiceUpload_Cancellation verifies that cancelling ctx during the
// resolve phase surfaces context.Canceled (wrapped) from Service.Upload.
func TestServiceUpload_Cancellation(t *testing.T) {
	mgr := mustNewService(t, Options{Resolver: ctxAwareResolver{}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.Upload(ctx, bytes.NewReader([]byte("data")), nil)
	if err == nil {
		t.Fatalf("Upload returned nil error; want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload err = %v; want errors.Is(err, context.Canceled)", err)
	}
}

// TestServiceCreateContexts_Cancellation verifies cancellation propagation
// through CreateContexts.
func TestServiceCreateContexts_Cancellation(t *testing.T) {
	mgr := mustNewService(t, Options{Resolver: ctxAwareResolver{}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.CreateContexts(ctx, nil)
	if err == nil {
		t.Fatalf("CreateContexts returned nil error; want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateContexts err = %v; want errors.Is(err, context.Canceled)", err)
	}
}

// ctxAwarePDPReader is a PDPVerifierReader that blocks on ctx.Done() so
// PieceStatus's parallel goroutines surface ctx.Err() from errgroup.Wait.
type ctxAwarePDPReader struct{}

func (ctxAwarePDPReader) GetScheduledRemovals(ctx context.Context, _ types.BigInt) ([]types.BigInt, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (ctxAwarePDPReader) FindPieceIdsByCid(ctx context.Context, _ types.BigInt, _ cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (ctxAwarePDPReader) GetNextChallengeEpoch(ctx context.Context, _ types.BigInt) (*big.Int, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (ctxAwarePDPReader) BlockNumber(ctx context.Context) (uint64, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

type ctxAwarePDPConfigReader struct{}

func (ctxAwarePDPConfigReader) GetPDPConfig(ctx context.Context) (*warmstorage.PDPConfig, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestContextPieceStatus_Cancellation verifies that when ctx is cancelled
// the parallel reads in PieceStatus surface context.Canceled.
func TestContextPieceStatus_Cancellation(t *testing.T) {
	c := mustPieceStatusContext(t, nil, nil)
	// Replace pdp readers with ctx-aware blockers.
	c.pdpCaller = ctxAwarePDPReader{}
	c.pdpConfig = ctxAwarePDPConfigReader{}

	pi, err := piecePCIDForTest()
	if err != nil {
		t.Fatalf("piece cid: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = c.PieceStatus(ctx, pi)
	if err == nil {
		t.Fatalf("PieceStatus returned nil error; want context.Canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PieceStatus err = %v; want errors.Is(err, context.Canceled)", err)
	}
}

func piecePCIDForTest() (cid.Cid, error) {
	return cid.Decode("bafkzcibcdpiwq7uxghoxc5wmpccnzqs36hyaiocy7yebbzlqcmxxljiroinwhig")
}
