package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSTerminator struct {
	gotDataSetID types.DataSetID
	res          *types.WriteResult
	err          error
	called       bool
}

func (f *fakeFWSSTerminator) TerminateDataSet(_ context.Context, id types.DataSetID, _ ...warmstorage.WriteOption) (*types.WriteResult, error) {
	f.called = true
	f.gotDataSetID = id
	return f.res, f.err
}

func TestContext_Terminate_NotConfigured(t *testing.T) {
	c, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.DataSetID(1)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err == nil {
		t.Fatal("expected error when terminator not configured")
	}
}

func TestContext_Terminate_Passthrough(t *testing.T) {
	term := &fakeFWSSTerminator{res: &types.WriteResult{Hash: common.HexToHash("0xdead")}}
	c, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.DataSetID(123)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	res, err := c.Terminate(context.Background())
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if !term.called || term.gotDataSetID != types.DataSetID(123) {
		t.Fatalf("terminator not invoked with expected id: called=%v id=%d", term.called, term.gotDataSetID)
	}
	if res == nil || res.Hash == (common.Hash{}) {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestContext_Terminate_PropagatesError(t *testing.T) {
	term := &fakeFWSSTerminator{err: errors.New("terminate failed")}
	c, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.DataSetID(1)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err == nil {
		t.Fatal("expected error from terminator")
	}
}

func TestContext_Terminate_TypedNilTerminatorTreatedAsUnset(t *testing.T) {
	var term *fakeFWSSTerminator

	c, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.DataSetID(1)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Terminate panicked with typed-nil terminator: %v", r)
		}
	}()

	_, err = c.Terminate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err=%v want not configured", err)
	}
}
