package integrationtest

import "github.com/strahe/synapse-go/chain"

// Funding margins for Prepare in integration flows. The shared test wallet has
// many active rails that drain available funds every epoch, and flows can run
// several epochs between Prepare and their last on-chain operation, so the
// SDK's default five-epoch buffer is too tight.
const (
	FundingExtraRunwayEpochs = chain.EpochsPerDay
	FundingBufferEpochs      = 120
)
