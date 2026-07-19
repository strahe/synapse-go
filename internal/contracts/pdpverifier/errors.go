package pdpverifier

import (
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	dataSetNotFoundSelector = pdpErrorSelector("DataSetNotFound()")
	dataSetNotLiveSelector  = pdpErrorSelector("DataSetNotLive()")
)

// IsDataSetUnavailable reports whether err is the PDPVerifier revert raised
// for terminated, missing, or otherwise non-readable data sets.
func IsDataSetUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "Data set not live") ||
		strings.Contains(msg, "DataSetNotLive") ||
		strings.Contains(msg, "DataSetNotFound") {
		return true
	}
	data, ok := ethclient.RevertErrorData(err)
	if !ok || len(data) < 4 {
		return false
	}
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	return selector == dataSetNotFoundSelector || selector == dataSetNotLiveSelector
}

func pdpErrorSelector(signature string) [4]byte {
	hash := crypto.Keccak256([]byte(signature))
	return [4]byte{hash[0], hash[1], hash[2], hash[3]}
}
