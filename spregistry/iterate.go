package spregistry

import (
	"context"
	"iter"

	"github.com/strahe/synapse-go/types"
)

// defaultIteratePageSize is used by IterateAll* helpers.
const defaultIteratePageSize uint64 = 50

// IterateAllPDPProviders yields every PDP provider across all pages. When
// onlyActive is true the registry filters inactive providers before applying
// pagination. The iterator terminates cleanly on ctx cancellation (yielding
// the context error), on RPC error, or when the range body returns false.
func (s *Service) IterateAllPDPProviders(ctx context.Context, onlyActive bool) iter.Seq2[PDPProvider, error] {
	return func(yield func(PDPProvider, error) bool) {
		var offset uint64
		for {
			if err := ctx.Err(); err != nil {
				yield(PDPProvider{}, err)
				return
			}
			page, err := s.GetPDPProviders(ctx, onlyActive, types.ListOptions{Offset: offset, Limit: defaultIteratePageSize})
			if err != nil {
				yield(PDPProvider{}, err)
				return
			}
			for _, p := range page.Providers {
				if !yield(p, nil) {
					return
				}
			}
			if !page.HasMore {
				return
			}
			offset += defaultIteratePageSize
		}
	}
}
