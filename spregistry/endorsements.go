package spregistry

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

// GetEndorsedProviderIDs returns the independently managed endorsed-provider
// set in contract order. Duplicate IDs are removed while preserving their
// first occurrence. Contract, RPC, and decoding errors are wrapped without
// changing their error identity.
//
// It returns ErrEndorsementsNotConfigured when Options.EndorsementsAddress
// was zero. A nil result with a nil error means the configured set is empty.
func (s *Service) GetEndorsedProviderIDs(ctx context.Context) ([]types.BigInt, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.endorsements == nil {
		return nil, ErrEndorsementsNotConfigured
	}

	raw, err := s.endorsements.GetProviderIds(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetEndorsedProviderIDs: %w", err)
	}
	ids, err := idconv.FromBigSlice("spregistry.GetEndorsedProviderIDs", raw)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(ids))
	unique := make([]types.BigInt, 0, len(ids))
	for _, id := range ids {
		key := idconv.Key(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, id)
	}
	return unique, nil
}
