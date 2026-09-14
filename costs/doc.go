// Package costs provides cost calculation for storage operations.
//
// It computes upload costs from the warmstorage PriceList, per-piece raw sizes,
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
// supplied current leaf count. An existing dataset requires a non-negative
// CurrentDataSetLeafCount; zero means known empty, while nil is invalid.
// Consequently, nil or empty single-target options return ErrInvalidArgument.
// Multi-target state comes from refs; nil options still uses defaults.
// Negative service runway and buffer values return ErrInvalidArgument.
//
// When migrating a single piece, wrap its raw size in []uint64{size}. For
// multiple pieces, supply the actual sizes: a total and count cannot recover
// per-piece rounding. Obtain current leaves from PDPVerifier, rather than
// converting a previously estimated byte size. Fees use the list length;
// CalculateUploadFees retains its standalone default of one for nil or
// non-positive piece counts and its minimum-batch fee assumption.
//
// The billing formula follows FilecoinServicesRef's PriceListUSDFC and Cids.
// Recheck it when upgrading the contract baseline. The pinned TypeScript
// v1.2.1 reference uses the previous size model; this calculation deliberately
// differs. Source alignment does not verify the deployed contract version.
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
