package storage

import (
	"context"
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

	if got != want {
		t.Fatalf("CreateContext returned %p want %p", got, want)
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
