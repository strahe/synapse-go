package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

type fakeContextResolver struct {
	providerFn func(context.Context, types.BigInt, NewProviderContextOptions) (*ProviderContext, error)
	dataSetFn  func(context.Context, types.BigInt, NewDataSetContextOptions) (*DataSetContext, error)
}

func (r *fakeContextResolver) ResolveProviderContext(ctx context.Context, id types.BigInt, opts NewProviderContextOptions) (*ProviderContext, error) {
	if r.providerFn == nil {
		return nil, errors.New("unexpected ResolveProviderContext")
	}
	return r.providerFn(ctx, id, opts)
}

func (r *fakeContextResolver) ResolveDataSetContext(ctx context.Context, id types.BigInt, opts NewDataSetContextOptions) (*DataSetContext, error) {
	if r.dataSetFn == nil {
		return nil, errors.New("unexpected ResolveDataSetContext")
	}
	return r.dataSetFn(ctx, id, opts)
}

type fakeContextSelector struct {
	providerFn func(context.Context, SelectProviderContextOptions) (*ProviderContext, error)
	uploadFn   func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error)
}

func (s *fakeContextSelector) SelectProviderContext(ctx context.Context, opts SelectProviderContextOptions) (*ProviderContext, error) {
	if s.providerFn == nil {
		return nil, errors.New("unexpected SelectProviderContext")
	}
	return s.providerFn(ctx, opts)
}

func (s *fakeContextSelector) SelectUploadContexts(ctx context.Context, opts SelectUploadContextsOptions) (*UploadContextSelection, error) {
	if s.uploadFn == nil {
		return nil, errors.New("unexpected SelectUploadContexts")
	}
	return s.uploadFn(ctx, opts)
}

func testProviderContextWithID(t *testing.T, id types.BigInt, identity ContextIdentity) *ProviderContext {
	t.Helper()
	provider := testProvider()
	provider.ID = id
	ctx, err := NewProviderContext(
		provider,
		&fakePDPProviderClient{},
		mustTestSigner(t),
		WithPayer(identity.Payer),
		WithChainID(identity.ChainID),
		WithRecordKeeper(identity.RecordKeeper),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	return ctx
}

func serviceTestIdentity() ContextIdentity {
	return ContextIdentity{Payer: testPayer(), ChainID: types.ChainID(314159), RecordKeeper: testRecordKeeper()}
}

func TestServiceNewProviderContextResolvesDefaultsAndCopiesInputs(t *testing.T) {
	svc := newTestService()
	svc.source = "app"
	svc.defaultWithCDN = true
	want := testProviderContextWithID(t, types.NewBigInt(9), serviceTestIdentity())
	metadata := map[string]string{"env": "prod"}
	withCDN := false

	svc.contextResolver = &fakeContextResolver{providerFn: func(_ context.Context, id types.BigInt, opts NewProviderContextOptions) (*ProviderContext, error) {
		if !id.Equal(types.NewBigInt(9)) {
			t.Fatalf("providerID=%s want 9", id.String())
		}
		if opts.DataSetMetadata["source"] != "app" || opts.DataSetMetadata["env"] != "prod" {
			t.Fatalf("metadata=%v", opts.DataSetMetadata)
		}
		if opts.WithCDN == nil || *opts.WithCDN {
			t.Fatalf("WithCDN=%v want false", opts.WithCDN)
		}
		metadata["env"] = "mutated"
		withCDN = true
		if opts.DataSetMetadata["env"] != "prod" || *opts.WithCDN {
			t.Fatal("resolver options alias caller-owned values")
		}
		return want, nil
	}}

	got, err := svc.NewProviderContext(context.Background(), types.NewBigInt(9), NewProviderContextOptions{
		DataSetMetadata: metadata,
		WithCDN:         &withCDN,
	})
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	if got != want {
		t.Fatalf("got=%p want=%p", got, want)
	}
}

func TestServiceNewDataSetContextValidatesReturnedTarget(t *testing.T) {
	svc := newTestService()
	providerID := testProvider().ID
	dataSetID := types.NewBigInt(42)
	want := mustWritableDataSetContext(t, &fakePDPProviderClient{}, testDataSetRef(dataSetID, types.BigInt{}))
	svc.contextResolver = &fakeContextResolver{dataSetFn: func(_ context.Context, id types.BigInt, opts NewDataSetContextOptions) (*DataSetContext, error) {
		if !id.Equal(dataSetID) || opts.ProviderID == nil || !opts.ProviderID.Equal(providerID) {
			t.Fatalf("id/options mismatch: %s %+v", id.String(), opts)
		}
		return want, nil
	}}

	got, err := svc.NewDataSetContext(context.Background(), dataSetID, NewDataSetContextOptions{ProviderID: &providerID})
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}
	if got != want {
		t.Fatalf("got=%p want=%p", got, want)
	}
}

func TestServiceContextConstructorsRejectWrongResolverTargets(t *testing.T) {
	identity := serviceTestIdentity()
	requestedProviderID := types.NewBigInt(9)

	t.Run("provider mismatch", func(t *testing.T) {
		svc := newTestService()
		svc.contextResolver = &fakeContextResolver{providerFn: func(context.Context, types.BigInt, NewProviderContextOptions) (*ProviderContext, error) {
			return testProviderContextWithID(t, types.NewBigInt(8), identity), nil
		}}
		result, err := svc.NewProviderContext(context.Background(), requestedProviderID, NewProviderContextOptions{})
		if result != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
		}
	})

	t.Run("data-set mismatch", func(t *testing.T) {
		svc := newTestService()
		wrong := mustWritableDataSetContext(t, &fakePDPProviderClient{}, testDataSetRef(types.NewBigInt(43), types.NewBigInt(1)))
		svc.contextResolver = &fakeContextResolver{dataSetFn: func(context.Context, types.BigInt, NewDataSetContextOptions) (*DataSetContext, error) {
			return wrong, nil
		}}
		result, err := svc.NewDataSetContext(context.Background(), types.NewBigInt(42), NewDataSetContextOptions{})
		if result != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
		}
	})

	t.Run("typed nil data set", func(t *testing.T) {
		svc := newTestService()
		svc.contextResolver = &fakeContextResolver{dataSetFn: func(context.Context, types.BigInt, NewDataSetContextOptions) (*DataSetContext, error) {
			return nil, nil
		}}
		result, err := svc.NewDataSetContext(context.Background(), types.NewBigInt(42), NewDataSetContextOptions{})
		if result != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
		}
	})
}

func TestServiceContextConstructorsRequireExplicitNonZeroIDs(t *testing.T) {
	svc := newTestService()
	for name, call := range map[string]func() error{
		"provider": func() error {
			_, err := svc.NewProviderContext(context.Background(), types.BigInt{}, NewProviderContextOptions{})
			return err
		},
		"data set": func() error {
			_, err := svc.NewDataSetContext(context.Background(), types.BigInt{}, NewDataSetContextOptions{})
			return err
		},
		"data-set provider assertion": func() error {
			zero := types.BigInt{}
			_, err := svc.NewDataSetContext(context.Background(), types.NewBigInt(1), NewDataSetContextOptions{ProviderID: &zero})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
		})
	}
}

func TestServiceSelectUploadContextsPreservesUsablePartialSelection(t *testing.T) {
	svc := newTestService()
	ctx1 := testProviderContextWithID(t, types.NewBigInt(1), serviceTestIdentity())
	svc.contextSelector = &fakeContextSelector{uploadFn: func(_ context.Context, opts SelectUploadContextsOptions) (*UploadContextSelection, error) {
		return &UploadContextSelection{
			Contexts:        []StorageContext{ctx1},
			RequestedCopies: opts.Copies,
			Complete:        false,
		}, &InsufficientUploadContextsError{Requested: opts.Copies, Available: 1}
	}}

	selection, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 2})
	if !errors.Is(err, ErrInsufficientUploadContexts) {
		t.Fatalf("error=%v want ErrInsufficientUploadContexts", err)
	}
	var insufficient *InsufficientUploadContextsError
	if !errors.As(err, &insufficient) || insufficient.Requested != 2 || insufficient.Available != 1 {
		t.Fatalf("error=%#v", err)
	}
	if selection == nil || len(selection.Contexts) != 1 || selection.Contexts[0] != ctx1 || selection.Complete {
		t.Fatalf("selection=%+v", selection)
	}
}

func TestServiceSelectUploadContextsDiscardsHardErrorResult(t *testing.T) {
	svc := newTestService()
	wantErr := errors.New("rpc failed")
	ctx1 := testProviderContextWithID(t, types.NewBigInt(1), serviceTestIdentity())
	svc.contextSelector = &fakeContextSelector{uploadFn: func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
		return &UploadContextSelection{Contexts: []StorageContext{ctx1}, RequestedCopies: 1, Complete: true}, wantErr
	}}

	selection, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 1})
	if selection != nil || !errors.Is(err, wantErr) {
		t.Fatalf("selection=%v error=%v", selection, err)
	}
}

func TestServiceSelectProviderContextRejectsNilCore(t *testing.T) {
	svc := newTestService()
	svc.contextSelector = &fakeContextSelector{providerFn: func(context.Context, SelectProviderContextOptions) (*ProviderContext, error) {
		return &ProviderContext{}, nil
	}}
	result, err := svc.SelectProviderContext(context.Background(), SelectProviderContextOptions{})
	if result != nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
	}
}

func TestServiceUploadToContextsIncompleteServiceIdentity(t *testing.T) {
	svc, err := New(Options{SignerAddress: testPayer()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	reader := &readCountingReader{}
	result, err := svc.UploadToContexts(context.Background(), reader, []StorageContext{&fakeUploadContext{id: types.NewBigInt(1)}}, nil)
	if result != nil || !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "service identity") || strings.Contains(err.Error(), "does not match") {
		t.Fatalf("result=%v error=%v want ErrInvalidArgument service identity", result, err)
	}
	if reader.reads != 0 {
		t.Fatalf("reader reads=%d want 0", reader.reads)
	}
}

func TestServiceSelectUploadContextsRejectsTypedNilAndIdentityMismatch(t *testing.T) {
	for _, tt := range []struct {
		name string
		ctx  StorageContext
	}{
		{name: "typed nil", ctx: (*ProviderContext)(nil)},
		{name: "nil core", ctx: &ProviderContext{}},
		{name: "wrong payer", ctx: testProviderContextWithID(t, types.NewBigInt(1), ContextIdentity{
			Payer: common.HexToAddress("0x9999"), ChainID: types.ChainID(314159), RecordKeeper: testRecordKeeper(),
		})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			svc.contextSelector = &fakeContextSelector{uploadFn: func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
				return &UploadContextSelection{Contexts: []StorageContext{tt.ctx}, RequestedCopies: 1, Complete: true}, nil
			}}
			selection, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 1})
			if selection != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("selection=%v error=%v", selection, err)
			}
		})
	}
}

func TestServiceSelectUploadContextsRejectsEveryIdentityMismatch(t *testing.T) {
	valid := serviceTestIdentity()
	tests := map[string]ContextIdentity{
		"zero payer":         {ChainID: valid.ChainID, RecordKeeper: valid.RecordKeeper},
		"wrong payer":        {Payer: common.HexToAddress("0x9999"), ChainID: valid.ChainID, RecordKeeper: valid.RecordKeeper},
		"zero chain":         {Payer: valid.Payer, RecordKeeper: valid.RecordKeeper},
		"wrong chain":        {Payer: valid.Payer, ChainID: types.ChainID(1), RecordKeeper: valid.RecordKeeper},
		"zero recordKeeper":  {Payer: valid.Payer, ChainID: valid.ChainID},
		"wrong recordKeeper": {Payer: valid.Payer, ChainID: valid.ChainID, RecordKeeper: common.HexToAddress("0x8888")},
	}
	for name, identity := range tests {
		t.Run(name, func(t *testing.T) {
			svc := newTestService()
			storageCtx := testProviderContextWithID(t, types.NewBigInt(1), identity)
			svc.contextSelector = &fakeContextSelector{uploadFn: func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
				return &UploadContextSelection{Contexts: []StorageContext{storageCtx}, RequestedCopies: 1, Complete: true}, nil
			}}
			selection, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 1})
			if selection != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("selection=%v error=%v", selection, err)
			}
		})
	}
}

func TestServiceSelectUploadContextsRejectsDuplicateProviderAndInconsistentResult(t *testing.T) {
	ctx1 := testProviderContextWithID(t, types.NewBigInt(1), serviceTestIdentity())
	for name, selectFn := range map[string]func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error){
		"duplicate provider": func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
			return &UploadContextSelection{Contexts: []StorageContext{ctx1, ctx1}, RequestedCopies: 2, Complete: true}, nil
		},
		"wrong requested copies": func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
			return &UploadContextSelection{Contexts: []StorageContext{ctx1}, RequestedCopies: 2, Complete: true}, nil
		},
		"partial without typed error": func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
			return &UploadContextSelection{Contexts: []StorageContext{ctx1}, RequestedCopies: 2, Complete: false}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := newTestService()
			svc.contextSelector = &fakeContextSelector{uploadFn: selectFn}
			selection, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 2})
			if selection != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("selection=%v error=%v", selection, err)
			}
		})
	}
}

func TestServiceSelectorsRejectExcludedProviderResults(t *testing.T) {
	providerID := types.NewBigInt(1)
	storageCtx := testProviderContextWithID(t, providerID, serviceTestIdentity())

	t.Run("provider", func(t *testing.T) {
		svc := newTestService()
		svc.contextSelector = &fakeContextSelector{providerFn: func(context.Context, SelectProviderContextOptions) (*ProviderContext, error) {
			return storageCtx, nil
		}}
		result, err := svc.SelectProviderContext(context.Background(), SelectProviderContextOptions{ExcludeProviderIDs: []types.BigInt{providerID}})
		if result != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
		}
	})

	t.Run("upload", func(t *testing.T) {
		svc := newTestService()
		svc.contextSelector = &fakeContextSelector{uploadFn: func(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error) {
			return &UploadContextSelection{Contexts: []StorageContext{storageCtx}, RequestedCopies: 1, Complete: true}, nil
		}}
		result, err := svc.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{
			Copies:             1,
			ExcludeProviderIDs: []types.BigInt{providerID},
		})
		if result != nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
		}
	})
}
