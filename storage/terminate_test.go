package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSTerminator struct {
	gotDataSetID types.BigInt
	res          *types.WriteResult
	err          error
	called       bool
}

func (f *fakeFWSSTerminator) TerminateDataSet(_ context.Context, id types.BigInt, _ ...warmstorage.WriteOption) (*types.WriteResult, error) {
	f.called = true
	f.gotDataSetID = id
	return f.res, f.err
}

func TestContext_Terminate_NotConfigured(t *testing.T) {
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
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
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(123)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	res, err := c.Terminate(context.Background())
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if !term.called || !term.gotDataSetID.Equal(types.NewBigInt(123)) {
		t.Fatalf("terminator not invoked with expected id: called=%v id=%s", term.called, term.gotDataSetID.String())
	}
	if res == nil || res.Hash == (common.Hash{}) {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestContext_Terminate_PropagatesError(t *testing.T) {
	term := &fakeFWSSTerminator{err: errors.New("terminate failed")}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
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

	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
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

func TestContext_Terminate_CanRunWhileDataSetCreationCompletes(t *testing.T) {
	dataSetID := types.NewBigInt(123)
	clientDataSetID := types.NewBigInt(456)
	txHash := common.HexToHash("0x1234")
	submission := CreateDataSetSubmission{
		TransactionID:   txHash.Hex(),
		StatusURL:       "https://sp.example.com/status",
		ClientDataSetID: &clientDataSetID,
	}
	client := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			if gotStatusURL != submission.StatusURL {
				return nil, errors.New("unexpected status URL")
			}
			id := dataSetID
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: txHash,
				DataSetID:         &id,
			}, nil
		},
	}
	term := &fakeFWSSTerminator{res: &types.WriteResult{Hash: common.HexToHash("0xdead")}}
	c, err := NewContext(testProvider(), client, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		for i := 0; i < 2000; i++ {
			if _, err := c.WaitForDataSetCreated(context.Background(), submission); err != nil {
				waitErr <- err
				return
			}
		}
		waitErr <- nil
	}()

	for i := 0; i < 2000; i++ {
		_, err := c.Terminate(context.Background())
		if err != nil && !strings.Contains(err.Error(), "dataSetID not set") {
			t.Fatalf("Terminate: %v", err)
		}
	}
	if err := <-waitErr; err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate after WaitForDataSetCreated: %v", err)
	}
}
