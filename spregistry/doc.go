// Package spregistry provides the Storage Provider Registry service.
//
// It queries the ServiceProviderRegistry contract to discover storage
// providers, their capabilities, endpoints, and endorsement status.
//
// Provider types:
//   - Endorsed: curated, high-quality providers (used as primary).
//   - Approved: automated QA-checked providers (used as secondary).
package spregistry
