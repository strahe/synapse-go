package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strahe/synapse-go/types"
)

func TestCreateContexts_InjectsSourceMetadata(t *testing.T) {
	svc := newTestService()
	svc.source = "app"
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var captured *UploadOptions
	svc.resolver = &fakeResolver{
		contexts: []UploadContext{ctx},
		captureFn: func(opts *UploadOptions) {
			captured = opts
		},
	}

	opts := &CreateContextsOptions{
		DataSetMetadata: map[string]string{"env": "prod"},
	}
	if _, err := svc.CreateContexts(context.Background(), opts); err != nil {
		t.Fatalf("CreateContexts: %v", err)
	}

	if captured == nil {
		t.Fatal("resolver did not receive options")
	}
	if got := captured.DataSetMetadata["source"]; got != "app" {
		t.Fatalf("captured source=%q want app", got)
	}
	if got := captured.DataSetMetadata["env"]; got != "prod" {
		t.Fatalf("captured env=%q want prod", got)
	}
	if _, ok := opts.DataSetMetadata["source"]; ok {
		t.Fatal("CreateContexts mutated caller metadata")
	}
}

func TestCreateContext_InjectsSourceMetadataWhenNilOptions(t *testing.T) {
	svc := newTestService()
	svc.source = "app"
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var captured *UploadOptions
	svc.resolver = &fakeResolver{
		contexts: []UploadContext{ctx},
		captureFn: func(opts *UploadOptions) {
			captured = opts
		},
	}

	if _, err := svc.CreateContext(context.Background(), nil); err != nil {
		t.Fatalf("CreateContext: %v", err)
	}

	if captured == nil {
		t.Fatal("resolver did not receive options")
	}
	if got := captured.DataSetMetadata["source"]; got != "app" {
		t.Fatalf("captured source=%q want app", got)
	}
	if captured.Copies != 1 {
		t.Fatalf("captured copies=%d want 1", captured.Copies)
	}
}

func TestCreateContext_ReturnsConcreteContext(t *testing.T) {
	svc := newTestService()
	want, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(7)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	svc.resolver = &fakeResolver{contexts: []UploadContext{want}}

	got, err := svc.CreateContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	_ = got.DeletePiece
	_ = got.DeletePieceByID

	if got != want {
		t.Fatalf("CreateContext returned %p want %p", got, want)
	}
}

func TestCreateContext_NonConcreteResolverContextIsPlainError(t *testing.T) {
	svc := newTestService()
	svc.resolver = &fakeResolver{contexts: []UploadContext{&fakeUploadContext{id: types.NewBigInt(1)}}}

	_, err := svc.CreateContext(context.Background(), nil)
	if err == nil {
		t.Fatal("CreateContext returned nil error; want non-*Context error")
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v should not match ErrInvalidArgument", err)
	}
}

func TestCreateContext_AcceptsProviderAssertionForDataSetID(t *testing.T) {
	svc := newTestService()
	dataSetID := types.NewBigInt(10)
	providerID := types.NewBigInt(1)
	want, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithDataSetID(dataSetID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var captured *UploadOptions
	svc.resolver = &fakeResolver{
		contexts: []UploadContext{want},
		captureFn: func(opts *UploadOptions) {
			captured = opts
		},
	}

	got, err := svc.CreateContext(context.Background(), &CreateContextOptions{
		DataSetID:  &dataSetID,
		ProviderID: &providerID,
	})
	if err != nil {
		t.Fatalf("CreateContext: %v", err)
	}
	if got != want {
		t.Fatalf("CreateContext returned %p want %p", got, want)
	}
	if captured == nil {
		t.Fatal("resolver did not receive options")
	}
	if len(captured.DataSetIDs) != 1 || !captured.DataSetIDs[0].Equal(dataSetID) {
		t.Fatalf("resolver DataSetIDs=%v want [%s]", captured.DataSetIDs, dataSetID.String())
	}
	if len(captured.ProviderIDs) != 0 {
		t.Fatalf("resolver ProviderIDs=%v want none for provider assertion", captured.ProviderIDs)
	}
	if captured.Copies != 1 {
		t.Fatalf("resolver Copies=%d want 1", captured.Copies)
	}
}

func TestCreateContext_RejectsMismatchedProviderAssertionForDataSetID(t *testing.T) {
	svc := newTestService()
	dataSetID := types.NewBigInt(10)
	providerID := types.NewBigInt(2)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithDataSetID(dataSetID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	svc.resolver = &fakeResolver{contexts: []UploadContext{ctx}}

	_, err = svc.CreateContext(context.Background(), &CreateContextOptions{
		DataSetID:  &dataSetID,
		ProviderID: &providerID,
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "DataSetID") || !strings.Contains(err.Error(), "ProviderID") {
		t.Fatalf("err=%v want DataSetID and ProviderID mismatch message", err)
	}
}

func TestCreateContext_RejectsZeroOptionIDs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		opts     *CreateContextOptions
		wantText string
	}{
		{
			name: "zero ProviderID",
			opts: func() *CreateContextOptions {
				id := types.NewBigInt(0)
				return &CreateContextOptions{ProviderID: &id}
			}(),
			wantText: "zero ProviderID",
		},
		{
			name: "zero DataSetID",
			opts: func() *CreateContextOptions {
				id := types.NewBigInt(0)
				return &CreateContextOptions{DataSetID: &id}
			}(),
			wantText: "zero DataSetID",
		},
		{
			name:     "zero ExcludeProviderID",
			opts:     &CreateContextOptions{ExcludeProviderIDs: []types.BigInt{types.NewBigInt(0)}},
			wantText: "zero ExcludeProviderID",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			resolverCalled := false
			svc.resolver = &fakeResolver{
				captureFn: func(*UploadOptions) {
					resolverCalled = true
				},
			}

			_, err := svc.CreateContext(context.Background(), tt.opts)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v want ErrInvalidArgument", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err=%v want %q", err, tt.wantText)
			}
			if resolverCalled {
				t.Fatal("resolver should not be called for zero ID")
			}
		})
	}
}

func TestCreateContextOptionsToUploadOptionsClonesCallerFields(t *testing.T) {
	dataSetMetadata := map[string]string{"env": "prod"}
	excludeProviderIDs := []types.BigInt{types.NewBigInt(2)}
	dataSetID := types.NewBigInt(10)
	opts := &CreateContextOptions{
		DataSetID:          &dataSetID,
		ExcludeProviderIDs: excludeProviderIDs,
		DataSetMetadata:    dataSetMetadata,
	}

	got := opts.toUploadOptions()
	opts.ExcludeProviderIDs[0] = types.NewBigInt(99)
	opts.DataSetMetadata["env"] = "dev"
	*opts.DataSetID = types.NewBigInt(77)

	if !got.ExcludeProviderIDs[0].Equal(types.NewBigInt(2)) {
		t.Fatalf("ExcludeProviderIDs[0]=%s want 2", got.ExcludeProviderIDs[0].String())
	}
	if got.DataSetMetadata["env"] != "prod" {
		t.Fatalf("DataSetMetadata[env]=%q want prod", got.DataSetMetadata["env"])
	}
	if !got.DataSetIDs[0].Equal(types.NewBigInt(10)) {
		t.Fatalf("DataSetIDs[0]=%s want 10", got.DataSetIDs[0].String())
	}
}

func TestCreateContexts_ReturnConcreteContexts(t *testing.T) {
	svc := newTestService()
	ctx1, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext #1: %v", err)
	}
	ctx2, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext #2: %v", err)
	}
	svc.resolver = &fakeResolver{contexts: []UploadContext{ctx1, ctx2}}

	got, err := svc.CreateContexts(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateContexts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got)=%d want 2", len(got))
	}
	_ = got[0].Terminate
	if got[0] != ctx1 || got[1] != ctx2 {
		t.Fatalf("CreateContexts returned unexpected contexts")
	}
}

func TestCreateContexts_NonConcreteResolverContextIsPlainError(t *testing.T) {
	svc := newTestService()
	svc.resolver = &fakeResolver{contexts: []UploadContext{&fakeUploadContext{id: types.NewBigInt(1)}}}

	_, err := svc.CreateContexts(context.Background(), nil)
	if err == nil {
		t.Fatal("CreateContexts returned nil error; want non-*Context error")
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v should not match ErrInvalidArgument", err)
	}
}

func TestCreateContexts_RejectsProviderIDsWithDataSetIDs(t *testing.T) {
	svc := newTestService()
	svc.resolver = &fakeResolver{}

	_, err := svc.CreateContexts(context.Background(), &CreateContextsOptions{
		ProviderIDs: []types.BigInt{types.NewBigInt(1)},
		DataSetIDs:  []types.BigInt{types.NewBigInt(2)},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
}

func TestCreateContextsOptionsToUploadOptionsClonesCallerFields(t *testing.T) {
	dataSetMetadata := map[string]string{"env": "prod"}
	providerIDs := []types.BigInt{types.NewBigInt(1)}
	dataSetIDs := []types.BigInt{types.NewBigInt(10)}
	excludeProviderIDs := []types.BigInt{types.NewBigInt(2)}
	opts := &CreateContextsOptions{
		ProviderIDs:        providerIDs,
		DataSetIDs:         dataSetIDs,
		ExcludeProviderIDs: excludeProviderIDs,
		DataSetMetadata:    dataSetMetadata,
	}

	got := opts.toUploadOptions()
	opts.ProviderIDs[0] = types.NewBigInt(99)
	opts.DataSetIDs[0] = types.NewBigInt(88)
	opts.ExcludeProviderIDs[0] = types.NewBigInt(77)
	opts.DataSetMetadata["env"] = "dev"

	if !got.ProviderIDs[0].Equal(types.NewBigInt(1)) {
		t.Fatalf("ProviderIDs[0]=%s want 1", got.ProviderIDs[0].String())
	}
	if !got.DataSetIDs[0].Equal(types.NewBigInt(10)) {
		t.Fatalf("DataSetIDs[0]=%s want 10", got.DataSetIDs[0].String())
	}
	if !got.ExcludeProviderIDs[0].Equal(types.NewBigInt(2)) {
		t.Fatalf("ExcludeProviderIDs[0]=%s want 2", got.ExcludeProviderIDs[0].String())
	}
	if got.DataSetMetadata["env"] != "prod" {
		t.Fatalf("DataSetMetadata[env]=%q want prod", got.DataSetMetadata["env"])
	}
}

func TestCreateContexts_RejectsZeroOptionIDs(t *testing.T) {
	for _, tt := range []struct {
		name     string
		opts     *CreateContextsOptions
		wantText string
	}{
		{
			name:     "zero ProviderID",
			opts:     &CreateContextsOptions{ProviderIDs: []types.BigInt{types.NewBigInt(0)}},
			wantText: "zero ProviderID",
		},
		{
			name:     "zero DataSetID",
			opts:     &CreateContextsOptions{DataSetIDs: []types.BigInt{types.NewBigInt(0)}},
			wantText: "zero DataSetID",
		},
		{
			name:     "zero ExcludeProviderID",
			opts:     &CreateContextsOptions{ExcludeProviderIDs: []types.BigInt{types.NewBigInt(0)}},
			wantText: "zero ExcludeProviderID",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			resolverCalled := false
			svc.resolver = &fakeResolver{
				captureFn: func(*UploadOptions) {
					resolverCalled = true
				},
			}

			_, err := svc.CreateContexts(context.Background(), tt.opts)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v want ErrInvalidArgument", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err=%v want %q", err, tt.wantText)
			}
			if resolverCalled {
				t.Fatal("resolver should not be called for zero ID")
			}
		})
	}
}
