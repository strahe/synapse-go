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
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// PieceStatus captures piece membership and data-set scheduling information.
// Estimated timing does not establish proof history or a missed proof.
type PieceStatus struct {
	// Exists reports whether the piece is currently present in the data set.
	Exists bool

	// PieceID is the piece's on-chain numeric id. Only meaningful when
	// Exists is true.
	PieceID types.BigInt

	// DataSetNextChallengeAt is the time of the scheduled challenge epoch,
	// which may be in the past. Zero time indicates unavailable timing.
	DataSetNextChallengeAt time.Time

	// DataSetEstimatedProofDeadline adds the configured window length to the
	// challenge epoch. It need not equal the contract's actual deadline.
	// Zero time indicates unavailable timing.
	DataSetEstimatedProofDeadline time.Time

	// RetrievalURL is the HTTPS URL at which the piece can be retrieved
	// from the provider. Empty when provider info is not available.
	RetrievalURL string

	// InEstimatedChallengeWindow reports whether the head epoch lies in the
	// estimated interval [challenge epoch, estimated deadline). False also
	// covers unavailable estimates and does not establish data-set health.
	InEstimatedChallengeWindow bool

	// HoursUntilNextChallenge is the nonnegative epoch difference in hours.
	// Zero also covers an unavailable challenge schedule.
	HoursUntilNextChallenge float64

	// EstimatedProofDeadlinePassed reports whether the head epoch has reached
	// the estimated deadline. False also covers unavailable estimates.
	EstimatedProofDeadlinePassed bool
}

// GetScheduledRemovals returns the list of piece ids that have been
// scheduled for removal from this data set but have not yet been processed.
// Missing or non-live data sets return [ErrDataSetUnavailable].
func (c *DataSetContext) GetScheduledRemovals(ctx context.Context) ([]types.BigInt, error) {
	if c.core.pdpCaller == nil {
		return nil, errors.New("storage.DataSetContext.GetScheduledRemovals: PDPVerifier reader not configured")
	}
	ids, err := c.core.pdpCaller.GetScheduledRemovals(ctx, c.ref.DataSetID())
	if err != nil {
		return nil, fmt.Errorf("storage.DataSetContext.GetScheduledRemovals: %w", err)
	}
	return ids, nil
}

// PieceStatus returns the current status of pieceCID relative to this
// context's data set and the challenge schedule. If the piece is not present
// in the data set, Exists is false and the other fields are zero-valued.
// pieceCID must be a PieceCIDv2; other CIDs return [ErrInvalidArgument].
// Missing or non-live data sets return [ErrDataSetUnavailable].
//
// Chain state and the head epoch are read independently, not at a fixed block.
// Missing or failed optional PDP configuration leaves estimates unavailable;
// the challenge time and remaining hours do not require that configuration.
// Other read failures and caller cancellation return wrapped errors.
func (c *DataSetContext) PieceStatus(ctx context.Context, pieceCID cid.Cid) (*PieceStatus, error) {
	if !pieceCID.Defined() {
		return nil, fmt.Errorf("storage.DataSetContext.PieceStatus: %w: undefined pieceCID", ErrInvalidArgument)
	}
	if _, err := piece.ParseV2(pieceCID); err != nil {
		return nil, fmt.Errorf("storage.DataSetContext.PieceStatus: %w: PieceCIDv2 required: %w", ErrInvalidArgument, err)
	}
	if c.core.pdpCaller == nil {
		return nil, errors.New("storage.DataSetContext.PieceStatus: PDPVerifier reader not configured")
	}

	var (
		pieceIDs           []types.BigInt
		nextChallengeEpoch *big.Int
		currentEpoch       uint64
		pdpConfig          *warmstorage.PDPConfig
		providerInfo       Provider
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		ids, err := c.core.pdpCaller.FindPieceIdsByCid(gctx, c.ref.DataSetID(), pieceCID, 0, 1)
		if err != nil {
			return fmt.Errorf("findPieceIdsByCid: %w", err)
		}
		pieceIDs = ids
		return nil
	})
	g.Go(func() error {
		n, err := c.core.pdpCaller.GetNextChallengeEpoch(gctx, c.ref.DataSetID())
		if err != nil {
			return fmt.Errorf("getNextChallengeEpoch: %w", err)
		}
		nextChallengeEpoch = n
		return nil
	})
	g.Go(func() error {
		n, err := c.core.pdpCaller.BlockNumber(gctx)
		if err != nil {
			return fmt.Errorf("blockNumber: %w", err)
		}
		currentEpoch = n
		return nil
	})
	g.Go(func() error {
		if c.core.pdpConfig == nil {
			return nil
		}
		cfg, err := c.core.pdpConfig.GetPDPConfig(gctx)
		if err != nil {
			// Optional configuration failures leave estimates unavailable.
			return nil //nolint:nilerr
		}
		pdpConfig = cfg
		return nil
	})
	g.Go(func() error {
		providerInfo = c.core.providerInfo()
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("storage.DataSetContext.PieceStatus: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("storage.DataSetContext.PieceStatus: %w", err)
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

	if nextChallengeEpoch == nil || nextChallengeEpoch.Sign() <= 0 {
		return out, nil
	}

	cur := new(big.Int).SetUint64(currentEpoch)
	chainResolved, err := chain.FromID(c.core.chainID.Int64())
	if err == nil {
		out.DataSetNextChallengeAt = chain.EpochToTime(chainResolved, nextChallengeEpoch)
	}
	if cur.Cmp(nextChallengeEpoch) < 0 {
		delta := new(big.Int).Sub(nextChallengeEpoch, cur)
		seconds := new(big.Int).Mul(delta, big.NewInt(chain.EpochDurationSeconds))
		if seconds.IsInt64() {
			out.HoursUntilNextChallenge = float64(seconds.Int64()) / 3600.0
		}
	}
	if pdpConfig == nil || pdpConfig.ChallengeWindowSize == nil || pdpConfig.ChallengeWindowSize.Sign() <= 0 {
		return out, nil
	}
	deadline := new(big.Int).Add(nextChallengeEpoch, pdpConfig.ChallengeWindowSize)
	if err == nil {
		out.DataSetEstimatedProofDeadline = chain.EpochToTime(chainResolved, deadline)
	}
	out.InEstimatedChallengeWindow = cur.Cmp(nextChallengeEpoch) >= 0 && cur.Cmp(deadline) < 0
	out.EstimatedProofDeadlinePassed = cur.Cmp(deadline) >= 0
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
