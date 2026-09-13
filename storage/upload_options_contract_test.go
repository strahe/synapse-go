package storage

import (
	"reflect"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

type optionField struct {
	name string
	typ  reflect.Type
}

func TestUploadOptionsPublicSurface(t *testing.T) {
	commonFields := []optionField{
		{name: "PieceMetadata", typ: reflect.TypeFor[map[string]string]()},
		{name: "PieceCID", typ: reflect.TypeFor[cid.Cid]()},
		{name: "OnProgress", typ: reflect.TypeFor[func(int64)]()},
		{name: "OnStored", typ: reflect.TypeFor[func(types.BigInt, cid.Cid)]()},
		{name: "OnPiecesAdded", typ: reflect.TypeFor[func(string, types.BigInt, []SubmittedPiece)]()},
		{name: "OnPiecesConfirmed", typ: reflect.TypeFor[func(types.BigInt, types.BigInt, []ConfirmedPiece)]()},
	}
	secondaryFields := []optionField{
		{name: "OnCopyComplete", typ: reflect.TypeFor[func(types.BigInt, cid.Cid)]()},
		{name: "OnCopyFailed", typ: reflect.TypeFor[func(types.BigInt, cid.Cid, error)]()},
		{name: "OnPullProgress", typ: reflect.TypeFor[func(types.BigInt, cid.Cid, PullStatus)]()},
	}

	tests := []struct {
		name   string
		typ    reflect.Type
		fields []optionField
	}{
		{
			name: "UploadOptions",
			typ:  reflect.TypeFor[UploadOptions](),
			fields: append([]optionField{
				{name: "Copies", typ: reflect.TypeFor[int]()},
				{name: "PieceMetadata", typ: reflect.TypeFor[map[string]string]()},
				{name: "DataSetMetadata", typ: reflect.TypeFor[map[string]string]()},
				{name: "ExcludeProviderIDs", typ: reflect.TypeFor[[]types.BigInt]()},
				{name: "AllowUnendorsedPrimary", typ: reflect.TypeFor[bool]()},
				{name: "WithCDN", typ: reflect.TypeFor[*bool]()},
				{name: "PieceCID", typ: reflect.TypeFor[cid.Cid]()},
				{name: "OnProgress", typ: reflect.TypeFor[func(int64)]()},
				{name: "OnStored", typ: reflect.TypeFor[func(types.BigInt, cid.Cid)]()},
				{name: "OnPiecesAdded", typ: reflect.TypeFor[func(string, types.BigInt, []SubmittedPiece)]()},
				{name: "OnPiecesConfirmed", typ: reflect.TypeFor[func(types.BigInt, types.BigInt, []ConfirmedPiece)]()},
			}, secondaryFields...),
		},
		{
			name:   "UploadToContextsOptions",
			typ:    reflect.TypeFor[UploadToContextsOptions](),
			fields: append(append([]optionField(nil), commonFields...), secondaryFields...),
		},
		{
			name:   "ContextUploadOptions",
			typ:    reflect.TypeFor[ContextUploadOptions](),
			fields: commonFields,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.typ.NumField() != len(tt.fields) {
				t.Fatalf("field count=%d want %d", tt.typ.NumField(), len(tt.fields))
			}
			for _, want := range tt.fields {
				got, ok := tt.typ.FieldByName(want.name)
				if !ok {
					t.Fatalf("missing field %s", want.name)
				}
				if got.Type != want.typ {
					t.Fatalf("field %s type=%s want %s", want.name, got.Type, want.typ)
				}
				if got.Anonymous || len(got.Index) != 1 {
					t.Fatalf("field %s is embedded or promoted; upload option types must remain flat", got.Name)
				}
				if got.PkgPath != "" {
					t.Fatalf("field %s is unexported", got.Name)
				}
			}
		})
	}
}

func TestNarrowUploadOptionConversionsOwnPieceMetadata(t *testing.T) {
	if uploadOptionsFromUploadToContexts(nil) != nil {
		t.Fatal("nil UploadToContextsOptions must remain nil")
	}
	if uploadOptionsFromContext(nil) != nil {
		t.Fatal("nil ContextUploadOptions must remain nil")
	}

	explicitMetadata := map[string]string{"kind": "explicit"}
	explicit := uploadOptionsFromUploadToContexts(&UploadToContextsOptions{
		PieceMetadata: explicitMetadata,
	})
	explicit.PieceMetadata["kind"] = "mutated"
	if explicitMetadata["kind"] != "explicit" {
		t.Fatal("UploadToContextsOptions PieceMetadata was shared with the upload pipeline")
	}

	contextMetadata := map[string]string{"kind": "context"}
	contextOpts := uploadOptionsFromContext(&ContextUploadOptions{
		PieceMetadata: contextMetadata,
	})
	contextOpts.PieceMetadata["kind"] = "mutated"
	if contextMetadata["kind"] != "context" {
		t.Fatal("ContextUploadOptions PieceMetadata was shared with the upload pipeline")
	}
}
