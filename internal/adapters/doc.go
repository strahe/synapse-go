// Package adapters contains the private glue that composes the per-service
// packages (warmstorage, spregistry, payments, costs, internal/contracts/
// pdpverifier) into the narrow interfaces consumed by the storage package.
//
// This package is a root-synapse implementation detail. It must not be
// imported outside the SDK and must not import the root synapse package.
// Each exported constructor returns a storage-package interface so that the
// concrete adapter types stay unexported and never enlarge the public API.
package adapters
