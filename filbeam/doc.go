// Package filbeam provides a client for the FilBeam stats API.
//
// FilBeam is Filecoin's pay-per-byte infrastructure. This client exposes
// remaining egress quota for FWSS data sets: CDN cache-hit egress and
// cache-miss egress (retrieval from storage providers).
//
// Usage:
//
//	svc := filbeam.NewService(chain.Calibration)
//	stats, err := svc.GetDataSetStats(ctx, big.NewInt(12345))
package filbeam
