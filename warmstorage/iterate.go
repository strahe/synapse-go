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

// GetAllClientDataSets returns every data set owned by payer across all pages.
// It materializes the full result set in memory; use IterateAllClientDataSets
// or GetClientDataSets with explicit pagination for large accounts.
func (s *Service) GetAllClientDataSets(ctx context.Context, payer common.Address) ([]*DataSetInfo, error) {
	var out []*DataSetInfo
	for info, err := range s.IterateAllClientDataSets(ctx, payer) {
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	if out == nil {
		out = []*DataSetInfo{}
	}
	return out, nil
}

// IterateAllClientDataSetIds yields every data set ID owned by payer
// across all pages. Semantics match IterateAllClientDataSets but returns
// a shallow list of IDs (lighter-weight than the full DataSetInfo).
func (s *Service) IterateAllClientDataSetIds(ctx context.Context, payer common.Address) iter.Seq2[types.BigInt, error] {
	return func(yield func(types.BigInt, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(types.BigInt{}, err)
				return
			}
			page, err := s.GetClientDataSetIds(ctx, payer, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(types.BigInt{}, err)
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

// GetAllClientDataSetIds returns every data set ID owned by payer across all pages.
// It materializes the full result set in memory; use IterateAllClientDataSetIds
// or GetClientDataSetIds with explicit pagination for large accounts.
func (s *Service) GetAllClientDataSetIds(ctx context.Context, payer common.Address) ([]types.BigInt, error) {
	var out []types.BigInt
	for id, err := range s.IterateAllClientDataSetIds(ctx, payer) {
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if out == nil {
		out = []types.BigInt{}
	}
	return out, nil
}

// IterateAllApprovedProviderIDs yields every approved-provider id across all
// pages. Semantics match IterateAllClientDataSets.
func (s *Service) IterateAllApprovedProviderIDs(ctx context.Context) iter.Seq2[types.BigInt, error] {
	return func(yield func(types.BigInt, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(types.BigInt{}, err)
				return
			}
			page, err := s.GetApprovedProviderIDs(ctx, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(types.BigInt{}, err)
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
