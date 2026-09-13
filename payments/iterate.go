package payments

import (
	"context"
	"fmt"
	"iter"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

const defaultIteratePageSize uint64 = 100

// IterateAllRailsAsPayer yields every rail charging payer for token across
// all pages. It fetches pages lazily and stops when the consumer stops or
// ctx is canceled. An error is yielded once, then iteration ends.
// Pages may observe different blocks; iteration is not an atomic snapshot.
func (s *Service) IterateAllRailsAsPayer(ctx context.Context, payer, token common.Address) iter.Seq2[RailListItem, error] {
	return s.iterateAllRails(ctx, payer, token, true)
}

// IterateAllRailsAsPayee yields every rail paying payee in token across
// all pages. It fetches pages lazily and stops when the consumer stops or
// ctx is canceled. An error is yielded once, then iteration ends.
// Pages may observe different blocks; iteration is not an atomic snapshot.
func (s *Service) IterateAllRailsAsPayee(ctx context.Context, payee, token common.Address) iter.Seq2[RailListItem, error] {
	return s.iterateAllRails(ctx, payee, token, false)
}

func (s *Service) iterateAllRails(ctx context.Context, account, token common.Address, asPayer bool) iter.Seq2[RailListItem, error] {
	op := "payments.IterateAllRailsAsPayer"
	if !asPayer {
		op = "payments.IterateAllRailsAsPayee"
	}
	return func(yield func(RailListItem, error) bool) {
		offset := new(big.Int)
		for {
			if err := ctx.Err(); err != nil {
				yield(RailListItem{}, err)
				return
			}
			page, err := s.listRails(ctx, account, token, asPayer, offset, defaultIteratePageSize)
			if err != nil {
				yield(RailListItem{}, err)
				return
			}
			if err := ctx.Err(); err != nil {
				yield(RailListItem{}, err)
				return
			}
			if page.NextOffset == nil || page.Total == nil {
				yield(RailListItem{}, fmt.Errorf("%s: %w: missing pagination values", op, ErrInvalidRailPage))
				return
			}
			hasMore := page.NextOffset.Cmp(page.Total) < 0
			if hasMore && page.NextOffset.Cmp(offset) <= 0 {
				yield(RailListItem{}, fmt.Errorf("%s: %w: next offset %s must advance beyond %s", op, ErrInvalidRailPage, page.NextOffset, offset))
				return
			}
			for _, rail := range page.Rails {
				if err := ctx.Err(); err != nil {
					yield(RailListItem{}, err)
					return
				}
				if !yield(rail, nil) {
					return
				}
			}
			if !hasMore {
				return
			}
			offset = page.NextOffset
		}
	}
}
