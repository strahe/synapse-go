package chain

var defaultRPCURLs = [chainCount]string{
	Mainnet:     "https://api.node.glif.io/rpc/v1",
	Calibration: "https://api.calibration.node.glif.io/rpc/v1",
}

// DefaultRPCURL returns the default public RPC endpoint for this chain.
// Returns an empty string for chains without a known RPC URL.
func (c Chain) DefaultRPCURL() string {
	if c < chainCount {
		return defaultRPCURLs[c]
	}
	return ""
}
