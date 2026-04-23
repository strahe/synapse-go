package spregistry

import sdktypes "github.com/strahe/synapse-go/types"

// Compile-time assertion: WriteResult must remain a type alias for
// types.WriteResult so cross-package assignments work without conversion.
var _ *WriteResult = (*sdktypes.WriteResult)(nil)
