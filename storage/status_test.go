package storage

import (
	"context"
	"errors"
	"math/big"
	"sync"
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

func (f *fakePDPReader) FindPieceIDsByCIDs(context.Context, types.BigInt, []cid.Cid) ([][]types.BigInt, error) {
	return nil, errors.New("unexpected FindPieceIDsByCIDs")
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

func mustPieceStatusContext(t *testing.T, pdp *fakePDPReader, cfg PDPConfigReader, opts ...ContextOption) *DataSetContext {
	t.Helper()
	options := []ContextOption{
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
		WithPDPConfigReader(cfg),
	}
	options = append(options, opts...)
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)), options...)
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

func TestContext_GetScheduledRemovals_NotConfigured(t *testing.T) {
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.GetScheduledRemovals(context.Background()); err == nil {
		t.Fatal("expected error when pdpCaller not configured")
	}
}

func TestContext_GetScheduledRemovals_UnavailableDataSet(t *testing.T) {
	c := mustPieceStatusContext(t, &fakePDPReader{scheduleErr: ErrDataSetUnavailable}, &fakePDPConfigReader{})
	if _, err := c.GetScheduledRemovals(context.Background()); !errors.Is(err, ErrDataSetUnavailable) {
		t.Fatalf("GetScheduledRemovals error = %v, want ErrDataSetUnavailable", err)
	}
}

func TestContext_PieceStatus_NotFound(t *testing.T) {
	pdp := &fakePDPReader{findIDs: nil, nextChallenge: big.NewInt(100), blockNumber: 50}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{MaxProvingPeriod: 120, ChallengeWindowSize: big.NewInt(30)}})
	st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
	if err != nil {
		t.Fatalf("PieceStatus: %v", err)
	}
	if st.Exists || !st.PieceID.IsZero() || st.RetrievalURL != "" || !st.DataSetNextChallengeAt.IsZero() ||
		!st.DataSetEstimatedProofDeadline.IsZero() || st.HoursUntilNextChallenge != 0 ||
		st.InEstimatedChallengeWindow || st.EstimatedProofDeadlinePassed {
		t.Fatalf("expected zero status for absent piece, got %+v", st)
	}
}

func TestContext_PieceStatus_EstimatedWindow(t *testing.T) {
	for _, tt := range []struct {
		name       string
		head       uint64
		inWindow   bool
		passed     bool
		untilHours float64
	}{
		{name: "before challenge", head: 40, untilHours: 0.5},
		{name: "at challenge", head: 100, inWindow: true},
		{name: "within estimated window", head: 110, inWindow: true},
		{name: "at estimated deadline", head: 160, passed: true},
		{name: "after estimated deadline", head: 161, passed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(99)}, nextChallenge: big.NewInt(100), blockNumber: tt.head}
			cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: big.NewInt(60)}}
			c := mustPieceStatusContext(t, pdp, cfg)
			pieceCID := mustPieceInfo(t).CIDv2
			st, err := c.PieceStatus(context.Background(), pieceCID)
			if err != nil {
				t.Fatalf("PieceStatus: %v", err)
			}
			if !st.Exists || !st.PieceID.Equal(types.NewBigInt(99)) || st.RetrievalURL != pieceRetrievalURL(testProvider().ServiceURL, pieceCID) {
				t.Fatalf("unexpected piece identity: %+v", st)
			}
			if st.InEstimatedChallengeWindow != tt.inWindow || st.EstimatedProofDeadlinePassed != tt.passed || st.HoursUntilNextChallenge != tt.untilHours {
				t.Fatalf("timing = %+v; want window=%v passed=%v hours=%v", st, tt.inWindow, tt.passed, tt.untilHours)
			}
		})
	}
}

func TestContext_PieceStatus_ScheduleTimes(t *testing.T) {
	for _, network := range []chain.Chain{chain.Mainnet, chain.Calibration} {
		t.Run(network.String(), func(t *testing.T) {
			pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(1)}, nextChallenge: big.NewInt(100), blockNumber: 110}
			cfg := &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: big.NewInt(60)}}
			c := mustPieceStatusContext(t, pdp, cfg, WithChainID(types.ChainID(network.ChainID())))
			for _, period := range []uint64{0, 20, 2880} {
				cfg.cfg.MaxProvingPeriod = period
				st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
				if err != nil {
					t.Fatalf("PieceStatus: %v", err)
				}
				if want := chain.EpochToTime(network, big.NewInt(100)); !st.DataSetNextChallengeAt.Equal(want) {
					t.Fatalf("period %d: challenge=%v want %v", period, st.DataSetNextChallengeAt, want)
				}
				if want := chain.EpochToTime(network, big.NewInt(160)); !st.DataSetEstimatedProofDeadline.Equal(want) {
					t.Fatalf("period %d: estimated deadline=%v want %v", period, st.DataSetEstimatedProofDeadline, want)
				}
			}
		})
	}
}

func TestContext_PieceStatus_OptionalConfig(t *testing.T) {
	var typedNil *fakePDPConfigReader
	for _, tt := range []struct {
		name string
		cfg  PDPConfigReader
	}{
		{name: "unset"},
		{name: "typed nil", cfg: typedNil},
		{name: "nil result", cfg: &fakePDPConfigReader{}},
		{name: "read failure", cfg: &fakePDPConfigReader{err: errors.New("unavailable")}},
		{name: "nil window", cfg: &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{}}},
		{name: "zero window", cfg: &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: new(big.Int)}}},
		{name: "negative window", cfg: &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: big.NewInt(-1)}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(1)}, nextChallenge: big.NewInt(100), blockNumber: 40}
			c := mustPieceStatusContext(t, pdp, tt.cfg)
			st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
			if err != nil {
				t.Fatalf("PieceStatus: %v", err)
			}
			if !st.Exists || !st.DataSetNextChallengeAt.Equal(chain.EpochToTime(chain.Calibration, big.NewInt(100))) || st.HoursUntilNextChallenge != 0.5 {
				t.Fatalf("expected direct schedule without config, got %+v", st)
			}
			if !st.DataSetEstimatedProofDeadline.IsZero() || st.InEstimatedChallengeWindow || st.EstimatedProofDeadlinePassed {
				t.Fatalf("expected unavailable estimates, got %+v", st)
			}
		})
	}
}

func TestContext_PieceStatus_NoChallenge(t *testing.T) {
	for _, challenge := range []*big.Int{nil, new(big.Int), big.NewInt(-1)} {
		t.Run(challenge.String(), func(t *testing.T) {
			pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(1)}, nextChallenge: challenge, blockNumber: 110}
			c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: big.NewInt(60)}})
			st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
			if err != nil {
				t.Fatalf("PieceStatus: %v", err)
			}
			if !st.Exists || !st.DataSetNextChallengeAt.IsZero() || !st.DataSetEstimatedProofDeadline.IsZero() || st.HoursUntilNextChallenge != 0 || st.InEstimatedChallengeWindow || st.EstimatedProofDeadlinePassed {
				t.Fatalf("expected existing piece without timing, got %+v", st)
			}
		})
	}
}

func TestContext_PieceStatus_UnknownChain(t *testing.T) {
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(1)}, nextChallenge: big.NewInt(100), blockNumber: 40}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{cfg: &warmstorage.PDPConfig{ChallengeWindowSize: big.NewInt(60)}}, WithChainID(types.ChainID(123)))
	for _, head := range []uint64{40, 110, 160} {
		pdp.blockNumber = head
		st, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2)
		if err != nil {
			t.Fatalf("PieceStatus: %v", err)
		}
		if !st.Exists || !st.DataSetNextChallengeAt.IsZero() || !st.DataSetEstimatedProofDeadline.IsZero() {
			t.Fatalf("unknown-chain wall-clock times must be unavailable: %+v", st)
		}
		if st.InEstimatedChallengeWindow != (head == 110) || st.EstimatedProofDeadlinePassed != (head == 160) {
			t.Fatalf("epoch comparisons unavailable at head %d: %+v", head, st)
		}
		if head == 40 && st.HoursUntilNextChallenge != 0.5 {
			t.Fatalf("hours=%v want 0.5", st.HoursUntilNextChallenge)
		}
	}
}

func TestContext_PieceStatus_PropagatesRequiredReadErrors(t *testing.T) {
	wantErr := errors.New("read failed")
	for _, tt := range []struct {
		name string
		pdp  *fakePDPReader
	}{
		{name: "find piece", pdp: &fakePDPReader{findErr: wantErr}},
		{name: "challenge schedule", pdp: &fakePDPReader{nextChallengeErr: wantErr}},
		{name: "head epoch", pdp: &fakePDPReader{blockNumberErr: wantErr}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := mustPieceStatusContext(t, tt.pdp, &fakePDPConfigReader{})
			if _, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2); !errors.Is(err, wantErr) {
				t.Fatalf("PieceStatus error=%v want %v", err, wantErr)
			}
		})
	}
}

func TestContext_PieceStatus_UnavailableDataSet(t *testing.T) {
	pdp := &fakePDPReader{
		findErr:          ErrDataSetUnavailable,
		nextChallengeErr: ErrDataSetUnavailable,
	}
	c := mustPieceStatusContext(t, pdp, &fakePDPConfigReader{})
	if _, err := c.PieceStatus(context.Background(), mustPieceInfo(t).CIDv2); !errors.Is(err, ErrDataSetUnavailable) {
		t.Fatalf("PieceStatus error = %v, want ErrDataSetUnavailable", err)
	}
}

type trackedPieceStatusReader struct {
	*fakePDPReader
	reads *sync.WaitGroup
}

func (f trackedPieceStatusReader) FindPieceIdsByCid(ctx context.Context, id types.BigInt, pieceCID cid.Cid, offset, limit uint64) ([]types.BigInt, error) {
	defer f.reads.Done()
	return f.fakePDPReader.FindPieceIdsByCid(ctx, id, pieceCID, offset, limit)
}

func (f trackedPieceStatusReader) GetNextChallengeEpoch(ctx context.Context, id types.BigInt) (*big.Int, error) {
	defer f.reads.Done()
	return f.fakePDPReader.GetNextChallengeEpoch(ctx, id)
}

func (f trackedPieceStatusReader) BlockNumber(ctx context.Context) (uint64, error) {
	defer f.reads.Done()
	return f.fakePDPReader.BlockNumber(ctx)
}

func TestContext_PieceStatus_CanceledWhileReadingOptionalConfig(t *testing.T) {
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(1)}, nextChallenge: big.NewInt(100), blockNumber: 40}
	c := mustPieceStatusContext(t, pdp, ctxAwarePDPConfigReader{})
	var reads sync.WaitGroup
	reads.Add(3)
	c.core.pdpCaller = trackedPieceStatusReader{fakePDPReader: pdp, reads: &reads}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	pieceCID := mustPieceInfo(t).CIDv2
	go func() {
		_, err := c.PieceStatus(ctx, pieceCID)
		result <- err
	}()
	reads.Wait()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("PieceStatus error=%v want context.Canceled", err)
	}
}
