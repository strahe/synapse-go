package payments

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	sdktypes "github.com/strahe/synapse-go/types"
)

// RailView is the flattened view of FilecoinPayV1RailView from the FilPay
// contract.
type RailView struct {
	Token               common.Address
	From                common.Address
	To                  common.Address
	Operator            common.Address
	Validator           common.Address
	PaymentRate         *big.Int
	LockupPeriod        *big.Int
	LockupFixed         *big.Int
	SettledUpTo         *big.Int
	EndEpoch            *big.Int
	CommissionRateBps   *big.Int
	ServiceFeeRecipient common.Address
}

// RailListItem is a single entry returned by GetRailsAsPayer /
// GetRailsAsPayee; it corresponds to `FilecoinPayV1RailInfo` on the
// contract side.
type RailListItem struct {
	RailID       sdktypes.BigInt
	IsTerminated bool
	EndEpoch     *big.Int
}

// RailPage is one page of rails plus pagination cursors.
type RailPage struct {
	Rails      []RailListItem
	NextOffset *big.Int
	Total      *big.Int
}

// GetRail returns the full view of a single rail by id.
func (s *Service) GetRail(ctx context.Context, railID sdktypes.BigInt) (*RailView, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if railID.IsZero() {
		return nil, fmt.Errorf("payments.GetRail: %w: railID must be > 0", ErrInvalidArgument)
	}
	v, err := s.filPayCall.GetRail(&bind.CallOpts{Context: ctx}, railID.Big())
	if err != nil {
		return nil, fmt.Errorf("payments.GetRail: %w", err)
	}
	return &RailView{
		Token:               v.Token,
		From:                v.From,
		To:                  v.To,
		Operator:            v.Operator,
		Validator:           v.Validator,
		PaymentRate:         copyBig(v.PaymentRate),
		LockupPeriod:        copyBig(v.LockupPeriod),
		LockupFixed:         copyBig(v.LockupFixed),
		SettledUpTo:         copyBig(v.SettledUpTo),
		EndEpoch:            copyBig(v.EndEpoch),
		CommissionRateBps:   copyBig(v.CommissionRateBps),
		ServiceFeeRecipient: v.ServiceFeeRecipient,
	}, nil
}

// ListOption tunes paginated list calls.
type ListOption func(*listConfig)

type listConfig struct {
	offset *big.Int
	limit  *big.Int
}

// WithListOffset sets the starting offset for paginated results.
func WithListOffset(offset *big.Int) ListOption {
	return func(c *listConfig) {
		if offset != nil {
			c.offset = new(big.Int).Set(offset)
		}
	}
}

// WithListLimit caps the number of results returned in one page.
// limit == 0 requests all remaining rails; negative values are ignored.
func WithListLimit(limit *big.Int) ListOption {
	return func(c *listConfig) {
		if limit != nil && limit.Sign() >= 0 {
			c.limit = new(big.Int).Set(limit)
		}
	}
}

func resolveListConfig(opts []ListOption) listConfig {
	cfg := listConfig{
		offset: big.NewInt(0),
		limit:  big.NewInt(0),
	}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// GetRailsAsPayer lists rails where `payer` is the account being charged
// for `token`. Callers may paginate with WithListOffset / WithListLimit.
func (s *Service) GetRailsAsPayer(ctx context.Context, payer, token common.Address, opts ...ListOption) (*RailPage, error) {
	return s.listRails(ctx, payer, token, true, opts)
}

// GetRailsAsPayee lists rails where `payee` is the account being paid
// on `token`. Callers may paginate with WithListOffset / WithListLimit.
func (s *Service) GetRailsAsPayee(ctx context.Context, payee, token common.Address, opts ...ListOption) (*RailPage, error) {
	return s.listRails(ctx, payee, token, false, opts)
}

func (s *Service) listRails(ctx context.Context, account, token common.Address, asPayer bool, opts []ListOption) (*RailPage, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	method := "GetRailsAsPayer"
	if !asPayer {
		method = "GetRailsAsPayee"
	}
	if (account == common.Address{}) {
		return nil, invalidZeroAddressError("payments."+method, "account")
	}
	if (token == common.Address{}) {
		return nil, invalidZeroAddressError("payments."+method, "token")
	}
	cfg := resolveListConfig(opts)
	call := &bind.CallOpts{Context: ctx}

	var nextOffset, total *big.Int
	items := []RailListItem{}
	if asPayer {
		out, err := s.filPayCall.GetRailsForPayerAndToken(call, account, token, cfg.offset, cfg.limit)
		if err != nil {
			return nil, fmt.Errorf("payments.%s: %w", method, err)
		}
		nextOffset, total = copyBig(out.NextOffset), copyBig(out.Total)
		for _, r := range out.Results {
			railID, err := idconv.FromBig("railID", r.RailId)
			if err != nil {
				return nil, fmt.Errorf("payments.%s: %w", method, err)
			}
			items = append(items, RailListItem{
				RailID:       railID,
				IsTerminated: r.IsTerminated,
				EndEpoch:     copyBig(r.EndEpoch),
			})
		}
	} else {
		out, err := s.filPayCall.GetRailsForPayeeAndToken(call, account, token, cfg.offset, cfg.limit)
		if err != nil {
			return nil, fmt.Errorf("payments.%s: %w", method, err)
		}
		nextOffset, total = copyBig(out.NextOffset), copyBig(out.Total)
		for _, r := range out.Results {
			railID, err := idconv.FromBig("railID", r.RailId)
			if err != nil {
				return nil, fmt.Errorf("payments.%s: %w", method, err)
			}
			items = append(items, RailListItem{
				RailID:       railID,
				IsTerminated: r.IsTerminated,
				EndEpoch:     copyBig(r.EndEpoch),
			})
		}
	}
	return &RailPage{Rails: items, NextOffset: nextOffset, Total: total}, nil
}
