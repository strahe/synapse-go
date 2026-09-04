// Package costs provides cost calculation for storage operations.
//
// It computes upload costs from the warmstorage PriceList, data set sizes,
// one-time operation fees, lockup requirements, and CDN options. The
// calculation matches on-chain Solidity integer division for accuracy.
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
// equal one hour, ~86 400 equal one month. On-chain lockup rates are
// per-epoch; effective monthly rates preserve monthly pricing precision for
// display and comparison.
//
// Basis points (bps) — one hundredth of one percent (1 bps = 0.01 %).
// Commission rates returned by warmstorage are expressed in basis points
// out of 10 000 (e.g. 500 bps = 5 %).
//
// Fees — one-time operation charges such as dataset creation and add-pieces
// submission fees. Add-pieces fees are counted per PDP batch.
//
// Lifecycle reserve — the flat lockup required when creating a dataset.
// It is separate from one-time operation fees.
//
// Lockup — funds reserved on the FilecoinPay contract to guarantee a
// stream of payments. Upload cost calculations include any additional lockup
// required by the new data and report the resulting deposit requirement.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package costs
