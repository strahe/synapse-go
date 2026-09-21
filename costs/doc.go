// Package costs provides cost calculation for storage operations.
//
// It computes upload costs from the warmstorage PriceList, per-piece raw sizes,
// lifecycle reserve state, one-time operation fees, lockup requirements, and
// CDN options. The calculation matches on-chain Solidity integer division for
// accuracy.
//
// # Entry points
//
//   - [Service.GetUploadCosts] — single-dataset upload cost.
//   - [Service.CalculateMultiContextCosts] — aggregated cost across multiple
//     upload contexts (one new + N existing data sets); used by the storage
//     manager's Prepare flow.
//   - [CalculateUploadFees] and [CalculateLifecycleReserveFunding] — pure
//     helpers for fee and reserve simulations.
//   - [PieceSizesToLeafCount] and [LeafCountToBillableBytes] — the size
//     conversions to use when composing [CalculateEffectiveRate] directly.
//
// # Piece sizes and existing state
//
// Service entry points accept a non-empty []uint64 of raw payload sizes,
// replicated to each target. Every size must be between chain.MinUploadSize
// and chain.MaxUploadSize. Added leaves are summed per piece using
// ceil(4*rawSize/127). Current and final aggregate leaf counts are converted
// to billable bytes using floor(leaves*32*127/128) before calculating rates.
// Leaf counts and aggregate sizes use arbitrary-precision integers.
//
// A new dataset requires UploadCostOptions.IsNewDataSet=true and ignores any
// supplied existing-dataset state. An existing dataset requires a non-negative
// CurrentDataSetLeafCount and CurrentLifecycleReserveBalance, plus a non-nil
// PDPEndEpoch that points to zero. PendingOneTimePayments defaults to zero and
// must otherwise be non-negative. A non-zero PDP end epoch returns
// [DataSetServiceTerminatedError]. Consequently, nil or empty single-target
// options return ErrInvalidArgument. Multi-target state comes from refs; nil
// options still uses defaults. Negative service runway and buffer values return
// ErrInvalidArgument.
//
// When migrating a single piece, wrap its raw size in []uint64{size}. For
// multiple pieces, supply the actual sizes: a total and count cannot recover
// per-piece rounding. Obtain current leaves from PDPVerifier, rather than
// converting a previously estimated byte size. Fee and reserve estimates
// conservatively treat each piece as a separate add-pieces operation because
// runtime batch boundaries are not known during estimation. Actual fees can be
// lower when pieces are submitted together.
//
// Lifecycle reserve funding is simulated in operation order. New data sets add
// the configured reserve target through AdditionalLockup.LifecycleLockup.
// Existing and new data sets add only conditional top-ups through
// AdditionalLockup.ReserveReplenishment. Upload fees remain visible in the
// result, but are not added directly to DepositNeeded because FWSS pays them
// from the reserve, reducing account funds and fixed lockup together.
//
// Multi-context calculations simulate each data set independently, then apply
// account debt, runway, available funds, and the execution buffer once to the
// aggregate.
//
// # Glossary
//
// Epoch — Filecoin block interval (30 seconds on mainnet and calibration).
// All on-chain rates and durations are denominated in epochs; 120 epochs
// equal one hour, ~86 400 equal one month. On-chain lockup rates are
// per-epoch; effective monthly rates preserve monthly pricing precision for
// display and comparison.
//
// Basis points (bps) — one hundredth of one percent (1 bps = 0.01 %).
// Commission rates returned by warmstorage are expressed in basis points
// out of 10 000 (e.g. 500 bps = 5 %).
//
// Fees — one-time operation charges such as dataset creation and add-pieces
// submission fees. Estimates conservatively charge one add-pieces base fee per
// piece and report the result separately from the required deposit.
//
// Lifecycle reserve — fixed lockup used by FWSS to pay lifecycle operation
// fees. A new dataset starts at the configured target; an active reserve is
// replenished only when pending fees plus the threshold exceed its balance.
//
// Lockup — funds reserved on the FilecoinPay contract to guarantee a
// stream of payments. Upload cost calculations include any additional lockup
// required by the new data and report the resulting deposit requirement.
package costs
