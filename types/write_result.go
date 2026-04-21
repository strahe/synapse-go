package types

import (
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// WriteResult is returned by every state-changing call in the SDK.
//
// Hash is always populated when WriteResult is non-nil — it is set as soon
// as the transaction is broadcast. Receipt is populated only when the call
// was made with a wait option (e.g. WithWait(timeout)) and the transaction
// was mined (successfully or reverted) before the timeout elapsed.
//
// Error semantics (observe WriteResult and err together):
//
//   - WriteResult == nil, err != nil: pre-broadcast failure (validation,
//     signing, or broadcast itself failed). The transaction was never
//     submitted to the chain and it is safe to retry with fresh state.
//   - WriteResult != nil, err == nil: transaction was broadcast
//     successfully. Without a wait option, Receipt is nil. With a wait
//     option, Receipt is non-nil and Status == 1 (successful execution).
//   - WriteResult != nil, err != nil, Receipt == nil: the wait timed out
//     before a terminal receipt was returned, or receipt lookup failed.
//     This includes both "not mined yet" and "already mined, but the
//     configured confirmation count has not been satisfied yet". Hash is
//     valid; callers can keep polling by Hash.
//   - WriteResult != nil, err != nil, Receipt != nil: transaction was
//     mined but execution reverted. err wraps ErrTxFailed (use errors.Is).
//     Receipt carries the failed status and any logs emitted before
//     revert.
type WriteResult struct {
	Hash    common.Hash
	Receipt *ethtypes.Receipt
}
