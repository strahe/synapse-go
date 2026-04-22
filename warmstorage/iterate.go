package warmstorage

import (
	"context"
	"iter"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

// defaultIteratePageSize is used by IterateAll* helpers. It is a SDK-side
// choice (not on-chain) so callers never need to pick a value.
const defaultIteratePageSize uint64 = 100

// IterateAllClientDataSets yields every data set owned by payer across all
// pages. It terminates cleanly when ctx is cancelled; the error yielded by
// the last (err != nil) pair is ctx.Err(). Break out of the range loop to
// stop early — the iterator performs no further RPC work after the body
// returns false.
func (s *Service) IterateAllClientDataSets(ctx context.Context, payer common.Address) iter.Seq2[*DataSetInfo, error] {
	return func(yield func(*DataSetInfo, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			page, err := s.GetClientDataSets(ctx, payer, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(nil, err)
				return
			}
			for _, ds := range page {
				if !yield(ds, nil) {
					return
				}
			}
			if uint64(len(page)) < defaultIteratePageSize {
				return
			}
			offset += uint64(len(page))
		}
	}
}

// IterateAllClientDataSetIds yields every data set ID owned by payer
// across all pages. Semantics match IterateAllClientDataSets but returns
// a shallow list of IDs (lighter-weight than the full DataSetInfo).
func (s *Service) IterateAllClientDataSetIds(ctx context.Context, payer common.Address) iter.Seq2[types.DataSetID, error] {
	return func(yield func(types.DataSetID, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(0, err)
				return
			}
			page, err := s.GetClientDataSetIds(ctx, payer, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(0, err)
				return
			}
			for _, id := range page {
				if !yield(id, nil) {
					return
				}
			}
			if uint64(len(page)) < defaultIteratePageSize {
				return
			}
			offset += uint64(len(page))
		}
	}
}

// IterateAllApprovedProviderIDs yields every approved-provider id across all
// pages. Semantics match IterateAllClientDataSets.
func (s *Service) IterateAllApprovedProviderIDs(ctx context.Context) iter.Seq2[types.ProviderID, error] {
	return func(yield func(types.ProviderID, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(0, err)
				return
			}
			page, err := s.GetApprovedProviderIDs(ctx, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(0, err)
				return
			}
			for _, id := range page {
				if !yield(id, nil) {
					return
				}
			}
			if uint64(len(page)) < defaultIteratePageSize {
				return
			}
			offset += uint64(len(page))
		}
	}
}
