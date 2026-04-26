// Package costs provides cost calculation for storage operations.
//
// It computes upload costs based on service pricing, data set sizes,
// effective rates, lockup requirements, and CDN options. The calculation
// matches on-chain Solidity integer division for accuracy.
//
// # Entry points
//
//   - [Service.GetUploadCosts] — single-dataset upload cost.
//   - [Service.CalculateMultiContextCosts] — aggregated cost across multiple
//     upload contexts (one new + N existing data sets); used by the storage
//     manager's Prepare flow.
//
// # Glossary
//
// Epoch — Filecoin block interval (30 seconds on mainnet and calibration).
// All on-chain rates and durations are denominated in epochs; 120 epochs
// equal one hour, ~86 400 equal one month. Monthly figures in this
// package are derived by multiplying per-epoch rates by
// [github.com/strahe/synapse-go/chain.EpochsPerMonth].
//
// Basis points (bps) — one hundredth of one percent (1 bps = 0.01 %).
// Commission rates returned by warmstorage are expressed in basis points
// out of 10 000 (e.g. 500 bps = 5 %).
//
// CDN fee — the optional egress fee charged when a dataset opts in to
// FilBeam CDN delivery. Applied on top of the base PDP rate and split
// between cache-hit and cache-miss rails.
//
// Sybil fee — a small flat USDFC amount that warmstorage locks on
// dataset creation to deter spam. Added to each upload cost estimate.
//
// Lockup — funds reserved on the FilecoinPay contract to guarantee a
// stream of payments. `LockupRatePerEpoch` is the per-epoch drain rate;
// `FundedUntilEpoch` is the epoch at which existing funds are exhausted
// at that rate.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package costs
