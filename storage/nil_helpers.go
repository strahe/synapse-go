package storage

import "github.com/strahe/synapse-go/internal/ifaceutil"

func normalizeOptional[T any](v T) T {
	return ifaceutil.NormalizeNil(v)
}
