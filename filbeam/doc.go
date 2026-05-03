// Package filbeam provides a client for the FilBeam stats API.
//
// FilBeam is Filecoin's pay-per-byte infrastructure. This client exposes
// remaining egress quota for FWSS data sets: CDN cache-hit egress and
// cache-miss egress (retrieval from storage providers).
//
// Usage:
//
//	svc, err := filbeam.New(filbeam.Options{Chain: chain.Calibration})
//	if err != nil {
//		// handle error (e.g. unsupported chain)
//	}
//	stats, err := svc.GetDataSetStats(ctx, types.NewBigInt(12345))
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package filbeam
