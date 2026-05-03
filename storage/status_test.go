package storage

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakePDPReader struct {
	scheduled   []types.BigInt
	scheduleErr error

	findIDs []types.BigInt
	findErr error

	nextChallenge    *big.Int
	nextChallengeErr error

	blockNumber    uint64
	blockNumberErr error
}

func (f *fakePDPReader) GetScheduledRemovals(_ context.Context, _ types.BigInt) ([]types.BigInt, error) {
	return f.scheduled, f.scheduleErr
}

func (f *fakePDPReader) FindPieceIdsByCid(_ context.Context, _ types.BigInt, _ cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	return f.findIDs, f.findErr
}

func (f *fakePDPReader) GetNextChallengeEpoch(_ context.Context, _ types.BigInt) (*big.Int, error) {
	return f.nextChallenge, f.nextChallengeErr
}

func (f *fakePDPReader) BlockNumber(_ context.Context) (uint64, error) {
	return f.blockNumber, f.blockNumberErr
}

type fakePDPConfigReader struct {
	cfg *warmstorage.PDPConfig
	err error
}

func (f *fakePDPConfigReader) GetPDPConfig(_ context.Context) (*warmstorage.PDPConfig, error) {
	return f.cfg, f.err
}

func mustPieceStatusContext(t *testing.T, pdp *fakePDPReader, cfg *fakePDPConfigReader) *Context {
	t.Helper()
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(7)),
		WithPDPVerifierReader(pdp),
		WithPDPConfigReader(cfg),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	return c
}

func TestContext_GetScheduledRemovals(t *testing.T) {
	pdp := &fakePDPReader{scheduled: []types.BigInt{types.NewBigInt(1), types.NewBigInt(2), types.NewBigInt(3)}}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{})
	got, err := c.GetScheduledRemovals(context.Background())
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 3 || !got[0].Equal(types.NewBigInt(1)) || !got[2].Equal(types.NewBigInt(3)) {
		t.Fatalf("unexpected scheduled removals: %v", got)
	}
}

func TestContext_GetScheduledRemovals_WithoutDataSetReturnsEmpty(t *testing.T) {
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(7)),
		WithPDPVerifierReader(&fakePDPReader{scheduled: []types.BigInt{types.NewBigInt(1), types.NewBigInt(2), types.NewBigInt(3)}}),
		WithPDPConfigReader(&fakePDPConfigReader{}),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := c.GetScheduledRemovals(context.Background())
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("scheduled removals=%v want empty", got)
	}
}

func TestContext_GetScheduledRemovals_NotConfigured(t *testing.T) {
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(7)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.GetScheduledRemovals(context.Background()); err == nil {
		t.Fatal("expected error when pdpCaller not configured")
	}
}

func TestContext_PieceStatus_NotFound(t *testing.T) {
	pdp := &fakePDPReader{findIDs: nil, nextChallenge: big.NewInt(100), blockNumber: 50}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 120, ChallengeWindowSize: big.NewInt(30)}})
	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
	if st.Exists {
		t.Fatalf("expected Exists=false, got %+v", st)
	}
}

func TestContext_PieceStatus_BeforeChallengeWindow(t *testing.T) {
	pdp := &fakePDPReader{
		findIDs:       []types.BigInt{types.NewBigInt(99)},
		nextChallenge: big.NewInt(1_000_000),
		blockNumber:   500_000,
	}
	cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 2880, ChallengeWindowSize: big.NewInt(60)}}
	c := mustPieceStatusContext(t, pdp, cfg)
	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
	if !st.Exists || !st.PieceID.Equal(types.NewBigInt(99)) {
		t.Fatalf("expected Exists=true PieceID=99, got %+v", st)
	}
	if st.InChallengeWindow || st.IsProofOverdue {
		t.Fatalf("expected outside challenge window, got %+v", st)
	}
	if st.HoursUntilChallengeWindow <= 0 {
		t.Fatalf("expected positive hoursUntilChallengeWindow, got %v", st.HoursUntilChallengeWindow)
	}
}

func TestContext_PieceStatus_InChallengeWindow(t *testing.T) {
	pdp := &fakePDPReader{
		findIDs:       []types.BigInt{types.NewBigInt(1)},
		nextChallenge: big.NewInt(100),
		blockNumber:   110,
	}
	cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 200, ChallengeWindowSize: big.NewInt(60)}}
	c := mustPieceStatusContext(t, pdp, cfg)
	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
	if !st.InChallengeWindow {
		t.Fatalf("expected InChallengeWindow=true, got %+v", st)
	}
	if st.IsProofOverdue {
		t.Fatalf("expected IsProofOverdue=false, got %+v", st)
	}
}

func TestContext_PieceStatus_Overdue(t *testing.T) {
	pdp := &fakePDPReader{
		findIDs:       []types.BigInt{types.NewBigInt(1)},
		nextChallenge: big.NewInt(100),
		blockNumber:   500,
	}
	cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 200, ChallengeWindowSize: big.NewInt(60)}}
	c := mustPieceStatusContext(t, pdp, cfg)
	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
	if !st.IsProofOverdue {
		t.Fatalf("expected IsProofOverdue=true, got %+v", st)
	}
}

func TestContext_PieceStatus_MainnetPopulatesProofTimes(t *testing.T) {
	pdp := &fakePDPReader{
		findIDs:       []types.BigInt{types.NewBigInt(1)},
		nextChallenge: big.NewInt(100),
		blockNumber:   110,
	}
	cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 20, ChallengeWindowSize: big.NewInt(60)}}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(7)),
		WithPDPVerifierReader(pdp),
		WithPDPConfigReader(cfg),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}

	wantDue := chain.EpochToTime(chain.Mainnet, big.NewInt(160))
	if !st.DataSetNextProofDue.Equal(wantDue) {
		t.Fatalf("DataSetNextProofDue=%v want %v", st.DataSetNextProofDue, wantDue)
	}
	wantLastProven := chain.EpochToTime(chain.Mainnet, big.NewInt(80))
	if !st.DataSetLastProven.Equal(wantLastProven) {
		t.Fatalf("DataSetLastProven=%v want %v", st.DataSetLastProven, wantLastProven)
	}
}

func TestContext_PieceStatus_PropagatesFindError(t *testing.T) {
	pdp := &fakePDPReader{findErr: errors.New("boom")}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{})
	if _, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2); err == nil {
		t.Fatal("expected error")
	}
}

func TestContext_PieceStatus_TypedNilPDPConfigReaderTreatedAsUnset(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{
		findIDs:       []types.BigInt{types.NewBigInt(1)},
		nextChallenge: big.NewInt(0),
		blockNumber:   0,
	}
	var cfg *fakePDPConfigReader

	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(7)),
		WithPDPVerifierReader(pdp),
		WithPDPConfigReader(cfg),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PieceStatus panicked with typed-nil PDP config reader: %v", r)
		}
	}()

	if _, err := c.PieceStatus(context.Background(), info.CIDv2); err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
}
