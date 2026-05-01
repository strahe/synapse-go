package storage

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSDataSetReader struct {
	calls   int
	gotID   types.DataSetID
	info    *warmstorage.DataSetInfo
	infoErr error
}

func (f *fakeFWSSDataSetReader) GetDataSet(_ context.Context, id types.DataSetID) (*warmstorage.DataSetInfo, error) {
	f.calls++
	f.gotID = id
	return f.info, f.infoErr
}

func newAutoFetchContext(t *testing.T, dataSetID types.DataSetID) *Context {
	t.Helper()
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), WithDataSetID(dataSetID))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	return c
}

func TestCreateContext_AutoFetchesClientDataSetID(t *testing.T) {
	const dsID types.DataSetID = 42
	wantClient := big.NewInt(0xC0FFEE)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: new(big.Int).Set(wantClient)},
	}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{newAutoFetchContext(t, dsID)}}

	got, err := svc.CreateContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader.calls=%d want 1", reader.calls)
	}
	if reader.gotID != dsID {
		t.Fatalf("reader.gotID=%d want %d", reader.gotID, dsID)
	}
	if got.clientDataSetID == nil || got.clientDataSetID.Cmp(wantClient) != 0 {
		t.Fatalf("clientDataSetID=%v want %v", got.clientDataSetID, wantClient)
	}
}

func TestCreateContext_AutoFetchTransientErrorIsSurfacedUnwrapped(t *testing.T) {
	boom := errors.New("rpc timeout")
	reader := &fakeFWSSDataSetReader{infoErr: boom}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{newAutoFetchContext(t, 7)}}

	_, err := svc.CreateContext(context.Background(), nil)
	if err == nil {
		t.Fatal("CreateContext: want error")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v want wrap of boom", err)
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v should NOT be classified as ErrInvalidArgument", err)
	}
}

func TestCreateContext_AutoFetchEmptyResultIsInvalidArgument(t *testing.T) {
	reader := &fakeFWSSDataSetReader{info: &warmstorage.DataSetInfo{ClientDataSetID: nil}}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{newAutoFetchContext(t, 7)}}

	_, err := svc.CreateContext(context.Background(), nil)
	if err == nil {
		t.Fatal("CreateContext: want error")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want wrap ErrInvalidArgument", err)
	}
}

func TestCreateContext_PrefersExplicitClientDataSetID(t *testing.T) {
	const dsID types.DataSetID = 11
	explicit := big.NewInt(0xABCD)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: big.NewInt(0xDEAD)},
	}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithDataSetID(dsID), WithClientDataSetID(explicit))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{c}}

	got, err := svc.CreateContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("reader.calls=%d want 0 (explicit value should win)", reader.calls)
	}
	if got.clientDataSetID == nil || got.clientDataSetID.Cmp(explicit) != 0 {
		t.Fatalf("clientDataSetID=%v want %v", got.clientDataSetID, explicit)
	}
}

func TestCreateContexts_AutoFetchCachesByDataSetID(t *testing.T) {
	const dsID types.DataSetID = 77
	wantClient := big.NewInt(0xCAFE)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: new(big.Int).Set(wantClient)},
	}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{
		contexts: []UploadContext{
			newAutoFetchContext(t, dsID),
			newAutoFetchContext(t, dsID),
		},
	}

	got, err := svc.CreateContexts(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateContexts: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader.calls=%d want 1 (duplicate dataSetID should share one FWSS lookup)", reader.calls)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2", len(got))
	}
	for i, ctx := range got {
		if ctx.clientDataSetID == nil || ctx.clientDataSetID.Cmp(wantClient) != 0 {
			t.Fatalf("got[%d].clientDataSetID=%v want %v", i, ctx.clientDataSetID, wantClient)
		}
	}
}

// TestPopulateClientDataSetIDsFromInterfaces_UploadPathParity asserts the
// Service.Upload-side wrapper iterates the resolver's []UploadContext,
// type-asserts to *Context, and back-fills clientDataSetID for matching
// concrete contexts. Non-*Context implementations are skipped silently
// so custom resolvers do not break.
func TestPopulateClientDataSetIDsFromInterfaces_UploadPathParity(t *testing.T) {
	const dsID types.DataSetID = 99
	wantClient := big.NewInt(0xBEEF)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: new(big.Int).Set(wantClient)},
	}
	svc := newTestService()
	svc.dsReader = reader

	concrete := newAutoFetchContext(t, dsID)
	contexts := []UploadContext{concrete, &fakeUploadContext{}}

	if err := svc.populateClientDataSetIDsFromInterfaces(context.Background(), contexts); err != nil {
		t.Fatalf("populateClientDataSetIDsFromInterfaces: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader.calls=%d want 1 (only the *Context should trigger a fetch)", reader.calls)
	}
	if concrete.clientDataSetID == nil || concrete.clientDataSetID.Cmp(wantClient) != 0 {
		t.Fatalf("clientDataSetID=%v want %v", concrete.clientDataSetID, wantClient)
	}
}
