package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"path"
	"time"

	"github.com/ipfs/go-cid"
	"golang.org/x/sync/errgroup"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

// PieceStatus mirrors the TypeScript PieceStatus shape and captures the
// current state of a piece relative to its data set's proving schedule.
//
// PieceID is zero when the piece is not present in the data set (Exists =
// false); callers should treat (Exists == false) as the "null" TS return.
type PieceStatus struct {
	// Exists reports whether the piece is currently present in the data set.
	Exists bool

	// PieceID is the piece's on-chain numeric id. Only meaningful when
	// Exists is true.
	PieceID uint64

	// DataSetLastProven is the wall-clock time of the most recent
	// proof submission for the enclosing data set. Zero time indicates
	// "never proven" or "unknown".
	DataSetLastProven time.Time

	// DataSetNextProofDue is the wall-clock deadline by which the next
	// proof must be submitted. Zero time indicates "unknown".
	DataSetNextProofDue time.Time

	// RetrievalURL is the HTTPS URL at which the piece can be retrieved
	// from the provider. Empty when provider info is not available.
	RetrievalURL string

	// InChallengeWindow reports whether the data set is currently inside
	// its challenge window (proof submission is required).
	InChallengeWindow bool

	// HoursUntilChallengeWindow is the number of hours between now and
	// the start of the next challenge window. Zero when inside or past
	// the window.
	HoursUntilChallengeWindow float64

	// IsProofOverdue reports whether the proving deadline has passed
	// without a proof being submitted.
	IsProofOverdue bool
}

// GetScheduledRemovals returns the list of piece ids that have been
// scheduled for removal from this data set but have not yet been
// processed. Matches TS StorageContext.getScheduledRemovals.
func (c *Context) GetScheduledRemovals(ctx context.Context) ([]uint64, error) {
	if c.pdpCaller == nil {
		return nil, errors.New("storage.Context.GetScheduledRemovals: PDPVerifier reader not configured")
	}
	if c.dataSetID == nil {
		return []uint64{}, nil
	}
	ids, err := c.pdpCaller.GetScheduledRemovals(ctx, *c.dataSetID)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.GetScheduledRemovals: %w", err)
	}
	return ids, nil
}

// PieceStatus returns the current status of pieceCID relative to this
// context's data set and the proving schedule. If the piece is not present
// in the data set, Exists is false and the other fields are zero-valued.
//
// The call performs up to five concurrent reads: findPieceIdsByCid,
// getNextChallengeEpoch, BlockNumber, getPDPConfig, and provider info.
// Any individual failure surfaces as a wrapped error; a nil pdpConfig is
// tolerated (proof-timing fields remain zero).
func (c *Context) PieceStatus(ctx context.Context, pieceCID cid.Cid) (*PieceStatus, error) {
	if c.pdpCaller == nil {
		return nil, errors.New("storage.Context.PieceStatus: PDPVerifier reader not configured")
	}
	if c.dataSetID == nil {
		return nil, fmt.Errorf("storage.Context.PieceStatus: %w: dataSetID not set", ErrInvalidArgument)
	}
	if !pieceCID.Defined() {
		return nil, fmt.Errorf("storage.Context.PieceStatus: %w: undefined pieceCID", ErrInvalidArgument)
	}

	var (
		pieceIDs           []uint64
		nextChallengeEpoch *big.Int
		currentEpoch       uint64
		pdpConfig          *warmstorage.PDPConfig
		providerInfo       Provider
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		ids, err := c.pdpCaller.FindPieceIdsByCid(gctx, *c.dataSetID, pieceCID, 0, 1)
		if err != nil {
			return fmt.Errorf("findPieceIdsByCid: %w", err)
		}
		pieceIDs = ids
		return nil
	})
	g.Go(func() error {
		n, err := c.pdpCaller.GetNextChallengeEpoch(gctx, *c.dataSetID)
		if err != nil {
			return fmt.Errorf("getNextChallengeEpoch: %w", err)
		}
		nextChallengeEpoch = n
		return nil
	})
	g.Go(func() error {
		n, err := c.pdpCaller.BlockNumber(gctx)
		if err != nil {
			return fmt.Errorf("blockNumber: %w", err)
		}
		currentEpoch = n
		return nil
	})
	g.Go(func() error {
		if c.pdpConfig == nil {
			return nil
		}
		cfg, err := c.pdpConfig.GetPDPConfig(gctx)
		if err != nil {
			// TS tolerates this failure; we do the same.
			return nil //nolint:nilerr
		}
		pdpConfig = cfg
		return nil
	})
	g.Go(func() error {
		providerInfo = c.GetProviderInfo()
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("storage.Context.PieceStatus: %w", err)
	}

	out := &PieceStatus{}
	if len(pieceIDs) == 0 {
		return out, nil
	}
	out.Exists = true
	out.PieceID = pieceIDs[0]
	if providerInfo.ServiceURL != "" {
		out.RetrievalURL = pieceRetrievalURL(providerInfo.ServiceURL, pieceCID)
	}

	if pdpConfig == nil || nextChallengeEpoch == nil || nextChallengeEpoch.Sign() == 0 {
		return out, nil
	}

	cur := new(big.Int).SetUint64(currentEpoch)
	window := pdpConfig.ChallengeWindowSize
	if window == nil {
		window = new(big.Int)
	}
	challengeStart := new(big.Int).Set(nextChallengeEpoch)
	provingDeadline := new(big.Int).Add(challengeStart, window)

	chainResolved, err := chain.FromID(c.chainID.Int64())
	if err == nil {
		out.DataSetNextProofDue = chain.EpochToTime(chainResolved, provingDeadline)
		if pdpConfig.MaxProvingPeriod > 0 {
			lastProvenEpoch := new(big.Int).Sub(challengeStart, new(big.Int).SetUint64(pdpConfig.MaxProvingPeriod))
			if lastProvenEpoch.Sign() > 0 {
				out.DataSetLastProven = chain.EpochToTime(chainResolved, lastProvenEpoch)
			}
		}
	}

	out.InChallengeWindow = cur.Cmp(challengeStart) >= 0 && cur.Cmp(provingDeadline) < 0
	out.IsProofOverdue = cur.Cmp(provingDeadline) >= 0
	if cur.Cmp(challengeStart) < 0 {
		delta := new(big.Int).Sub(challengeStart, cur)
		seconds := new(big.Int).Mul(delta, big.NewInt(chain.EpochDurationSeconds))
		if seconds.IsInt64() {
			out.HoursUntilChallengeWindow = float64(seconds.Int64()) / 3600.0
		}
	}
	return out, nil
}

func pieceRetrievalURL(serviceURL string, pieceCID cid.Cid) string {
	base, err := url.Parse(serviceURL)
	if err != nil {
		return serviceURL
	}
	base.Path = path.Join(base.Path, "piece", pieceCID.String())
	return base.String()
}
