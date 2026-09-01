package storage_test

import (
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/storage"
)

var _ storage.PDPProviderClient = (*pdp.Client)(nil)
