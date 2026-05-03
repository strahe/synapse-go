package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSDataSetReader struct {
	calls   int
	gotID   types.BigInt
	info    *warmstorage.DataSetInfo
	infoErr error
}

func (f *fakeFWSSDataSetReader) GetDataSet(_ context.Context, id types.BigInt) (*warmstorage.DataSetInfo, error) {
	f.calls++
	f.gotID = id
	return f.info, f.infoErr
}

func newAutoFetchContext(t *testing.T, dataSetID types.BigInt) *Context {
	t.Helper()
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), WithDataSetID(dataSetID))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	return c
}

func TestCreateContext_AutoFetchesClientDataSetID(t *testing.T) {
	dsID := types.NewBigInt(42)
	wantClient := types.NewBigInt(0xC0FFEE)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: wantClient},
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
	if !reader.gotID.Equal(dsID) {
		t.Fatalf("reader.gotID=%s want %s", reader.gotID.String(), dsID.String())
	}
	if got.clientDataSetID == nil || !got.clientDataSetID.Equal(wantClient) {
		t.Fatalf("clientDataSetID=%v want %s", got.clientDataSetID, wantClient.String())
	}
}

func TestCreateContext_AutoFetchTransientErrorIsSurfacedUnwrapped(t *testing.T) {
	boom := errors.New("rpc timeout")
	reader := &fakeFWSSDataSetReader{infoErr: boom}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{newAutoFetchContext(t, types.NewBigInt(7))}}

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
	reader := &fakeFWSSDataSetReader{}
	svc := newTestService()
	svc.dsReader = reader
	svc.resolver = &fakeResolver{contexts: []UploadContext{newAutoFetchContext(t, types.NewBigInt(7))}}

	_, err := svc.CreateContext(context.Background(), nil)
	if err == nil {
		t.Fatal("CreateContext: want error")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want wrap ErrInvalidArgument", err)
	}
}

func TestCreateContext_PrefersExplicitClientDataSetID(t *testing.T) {
	dsID := types.NewBigInt(11)
	explicit := types.NewBigInt(0xABCD)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: types.NewBigInt(0xDEAD)},
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
	if got.clientDataSetID == nil || !got.clientDataSetID.Equal(explicit) {
		t.Fatalf("clientDataSetID=%v want %s", got.clientDataSetID, explicit.String())
	}
}

func TestCreateContexts_AutoFetchCachesByDataSetID(t *testing.T) {
	dsID := types.NewBigInt(77)
	wantClient := types.NewBigInt(0xCAFE)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: wantClient},
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
		if ctx.clientDataSetID == nil || !ctx.clientDataSetID.Equal(wantClient) {
			t.Fatalf("got[%d].clientDataSetID=%v want %s", i, ctx.clientDataSetID, wantClient.String())
		}
	}
}

// TestPopulateClientDataSetIDsFromInterfaces_UploadPathParity asserts the
// Service.Upload-side wrapper iterates the resolver's []UploadContext,
// type-asserts to *Context, and back-fills clientDataSetID for matching
// concrete contexts. Non-*Context implementations are skipped silently
// so custom resolvers do not break.
func TestPopulateClientDataSetIDsFromInterfaces_UploadPathParity(t *testing.T) {
	dsID := types.NewBigInt(99)
	wantClient := types.NewBigInt(0xBEEF)

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: wantClient},
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
	if concrete.clientDataSetID == nil || !concrete.clientDataSetID.Equal(wantClient) {
		t.Fatalf("clientDataSetID=%v want %s", concrete.clientDataSetID, wantClient.String())
	}
}
