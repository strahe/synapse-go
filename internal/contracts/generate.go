package contracts

// This file only owns abigen directives.
// Always run `make generate-contracts` before regenerating bindings.

//go:generate abigen --abi pdpverifier/abi.json --pkg pdpverifier --type PDPVerifier --out pdpverifier/bindings.go
//go:generate abigen --abi fwss/abi.json --pkg fwss --type FWSS --out fwss/bindings.go
//go:generate abigen --abi fwssview/abi.json --pkg fwssview --type FWSSView --out fwssview/bindings.go
//go:generate abigen --abi spregistry/abi.json --pkg spregistry --type SPRegistry --out spregistry/bindings.go
//go:generate abigen --abi filpay/abi.json --pkg filpay --type FilPay --out filpay/bindings.go
//go:generate abigen --abi erc20/abi.json --pkg erc20 --type ERC20 --out erc20/bindings.go
//go:generate abigen --abi sessionkeyregistry/abi.json --pkg sessionkeyregistry --type SessionKeyRegistry --out sessionkeyregistry/bindings.go
