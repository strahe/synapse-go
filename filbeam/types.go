package filbeam

import "math/big"

// DataSetStats contains the remaining pay-per-byte egress quotas for a
// FilBeam data set. Values decrease as data is served and represent how
// many bytes can still be retrieved before adding more credits.
type DataSetStats struct {
	// CDNEgressQuota is the remaining bytes that can be served from
	// FilBeam's cache (fast, direct CDN delivery — cache hits).
	CDNEgressQuota *big.Int

	// CacheMissEgressQuota is the remaining bytes that can be retrieved
	// from storage providers (triggers caching on first fetch).
	CacheMissEgressQuota *big.Int
}
