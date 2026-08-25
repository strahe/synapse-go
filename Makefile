.PHONY: build test test-cover test-cover-public test-race bench lint vet generate generate-contracts clean fmt tidy check check-ts-baseline test-integration test-integration-readonly test-integration-fast test-integration-cross

INTEGRATION_PKGS := ./costs ./payments ./sessionkey ./spregistry ./storage ./tests/integration ./warmstorage
INTEGRATION_READONLY_PKGS := ./costs ./spregistry ./warmstorage
INTEGRATION_FAST_RUN := ^TestIntegration$$/(Costs|Payments|Upload|Download|ClientSmoke|StorageManagerSurface|ContextInspection|WarmStorageInspection|StorageLifecycle)$$
PUBLIC_PKGS := . ./chain ./signer ./piece ./types ./payments ./warmstorage ./spregistry ./sessionkey ./filbeam ./pdp ./costs ./storage

# Default target
all: check

# Build all packages
build:
	go build ./...

# Run all tests
test:
	go test ./...

# Run benchmarks — auto-discovers packages containing *_bench_test.go files.
# Skip hidden dirs (worktrees, .git) and local reference checkouts.
bench:
	go test -bench=. -benchmem $(shell find . \( -path '*/.*' -o -path './lotus' -o -path './curio' -o -path './synapse-sdk' -o -path './go-synapse' \) -prune -o -name '*_bench_test.go' -print | sed 's|/[^/]*$$||' | sort -u)

# Run tests with race detector
test-race:
	go test -race ./...

# Run tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run coverage for the supported SDK surface only. Generated bindings,
# examples and internal tools remain visible through test-cover.
test-cover-public:
	go test -coverprofile=coverage.public.out $(PUBLIC_PKGS)
	go tool cover -html=coverage.public.out -o coverage.public.html
	@awk -F'[: ,]+' 'NR > 1 { \
		pkg = $$1; sub("/[^/]+$$", "", pkg); statements = $$(NF-1); \
		total[pkg] += statements; aggregateTotal += statements; \
		if ($$NF > 0) { covered[pkg] += statements; aggregateCovered += statements } \
	} END { \
		failed = 0; \
		for (pkg in total) { \
			pct = 100 * covered[pkg] / total[pkg]; \
			if (pct + 0.000001 < 80) { printf "coverage below 80%%: %s %.2f%%\n", pkg, pct; failed = 1 } \
		} \
		aggregate = 100 * aggregateCovered / aggregateTotal; \
		printf "public weighted coverage: %.3f%%\n", aggregate; \
		if (aggregate + 0.000001 < 87) { printf "public weighted coverage is below 87%%\n"; failed = 1 } \
		exit failed \
	}' coverage.public.out

# Run the full integration suite (requires env vars).
# -tags adds integration-only files to the build.
# -run restricts execution to integration entry points so unit tests do not also run.
# -p 1 is required when using a single shared wallet, otherwise package-level
# parallelism races on FEVM nonces and causes mpool conflicts.
test-integration:
	go test -tags=integration -run '^TestIntegration' -p 1 -count=1 -v -timeout 60m $(INTEGRATION_PKGS)

# Run read-only integration tests. These packages do not broadcast
# transactions, so package-level parallelism is safe with one wallet.
test-integration-readonly:
	go test -tags=integration -run '^TestIntegration' -p 3 -count=1 -v -timeout 10m $(INTEGRATION_READONLY_PKGS)

# Run a faster single-wallet smoke path. Read-only packages can run in
# parallel; the cross-package flow stays serial and includes dataset cleanup.
test-integration-fast: test-integration-readonly
	go test -tags=integration -run '$(INTEGRATION_FAST_RUN)' -p 1 -count=1 -v -timeout 30m ./tests/integration

# Run only the cross-package integration flows under tests/integration.
test-integration-cross:
	go test -tags=integration -run '^TestIntegration$$' -count=1 -v -timeout 20m ./tests/integration

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Format code
fmt:
	gofumpt -extra -w .

# Tidy modules
tidy:
	go mod tidy

# Generate code (contract bindings, etc.)
generate-contracts:
	go run ./internal/contracts/cmd/syncabi
	go generate ./internal/contracts/...

generate:
	$(MAKE) generate-contracts

# Check the ignored local synapse-sdk/ checkout, package layering, and ABI ref.
check-ts-baseline:
	CHECK_TS_SDK_BASELINE=1 go test ./internal/upstream -run '^TestLocalTSSDKBaseline$$' -count=1

# Run all checks (build + vet + lint + test)
check: build vet lint test

# Clean build artifacts
clean:
	rm -f coverage.out coverage.html coverage.public.out coverage.public.html
	go clean ./...
