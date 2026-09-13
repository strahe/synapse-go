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

// RailPage is one page of rails plus pagination cursors. A page can contain
// fewer rails than the requested limit because finalized slots are skipped.
type RailPage struct {
	Rails []RailListItem
	// NextOffset is the continuation offset returned by the contract.
	// There are more slots when NextOffset < Total. Check IsUint64 before
	// converting it to ListOptions.Offset, or use IterateAllRailsAsPayer/Payee.
	NextOffset *big.Int
	// Total is the number of underlying rail slots, including finalized slots.
	Total *big.Int
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

// GetRailsAsPayer reads one page of rails charging payer for token.
// opts.Limit must be > 0 and bounds the slots examined, not the number of
// returned rails. Use IterateAllRailsAsPayer for unbounded traversal.
func (s *Service) GetRailsAsPayer(ctx context.Context, payer, token common.Address, opts sdktypes.ListOptions) (*RailPage, error) {
	return s.listRails(ctx, payer, token, true, new(big.Int).SetUint64(opts.Offset), opts.Limit)
}

// GetRailsAsPayee reads one page of rails paying payee in token.
// opts.Limit must be > 0 and bounds the slots examined, not the number of
// returned rails. Use IterateAllRailsAsPayee for unbounded traversal.
func (s *Service) GetRailsAsPayee(ctx context.Context, payee, token common.Address, opts sdktypes.ListOptions) (*RailPage, error) {
	return s.listRails(ctx, payee, token, false, new(big.Int).SetUint64(opts.Offset), opts.Limit)
}

func (s *Service) listRails(ctx context.Context, account, token common.Address, asPayer bool, offset *big.Int, limit uint64) (*RailPage, error) {
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
	if err := (sdktypes.ListOptions{Limit: limit}).Validate(); err != nil {
		return nil, fmt.Errorf("payments.%s: %w: %w", method, ErrInvalidArgument, err)
	}
	call := &bind.CallOpts{Context: ctx}
	limitBig := new(big.Int).SetUint64(limit)

	var nextOffset, total *big.Int
	items := []RailListItem{}
	if asPayer {
		out, err := s.filPayCall.GetRailsForPayerAndToken(call, account, token, offset, limitBig)
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
		out, err := s.filPayCall.GetRailsForPayeeAndToken(call, account, token, offset, limitBig)
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
