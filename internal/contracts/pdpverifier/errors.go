package pdpverifier

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
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
	data, ok := ethclient.RevertErrorData(err)
	if ok && len(data) >= 4 {
		return IsDataSetUnavailableData(data)
	}
	return isDataSetUnavailableText(err.Error())
}

// IsDataSetUnavailableData reports whether raw EVM revert data encodes one of
// the known PDPVerifier unavailable-data-set custom errors or its legacy
// Error(string) reason. Unknown decodable reverts are never classified by
// surrounding error text.
func IsDataSetUnavailableData(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	if selector == dataSetNotFoundSelector || selector == dataSetNotLiveSelector {
		return true
	}
	reason, err := abi.UnpackRevert(data)
	return err == nil && isDataSetUnavailableText(reason)
}

func isDataSetUnavailableText(message string) bool {
	message = strings.TrimSpace(message)
	knownSuffix := strings.HasSuffix(message, "DataSetNotLive()") ||
		strings.HasSuffix(message, "DataSetNotFound()") ||
		strings.HasSuffix(message, "Data set not live")
	if !knownSuffix {
		return false
	}
	return message == "DataSetNotLive()" ||
		message == "DataSetNotFound()" ||
		message == "Data set not live" ||
		strings.Contains(strings.ToLower(message), "execution reverted")
}

func pdpErrorSelector(signature string) [4]byte {
	hash := crypto.Keccak256([]byte(signature))
	return [4]byte{hash[0], hash[1], hash[2], hash[3]}
}
