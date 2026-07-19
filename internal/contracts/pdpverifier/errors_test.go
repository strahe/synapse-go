package pdpverifier

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type revertDataError struct {
	data string
}

func (e revertDataError) Error() string {
	return "execution reverted"
}

func (e revertDataError) ErrorCode() int {
	return 3
}

func (e revertDataError) ErrorData() interface{} {
	return e.data
}

func customError(name string) error {
	hash := crypto.Keccak256([]byte(name + "()"))
	return revertDataError{data: hexutil.Encode(hash[:4])}
}

func TestIsDataSetUnavailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "ordinary error", err: errors.New("connection reset"), want: false},
		{name: "legacy not live string", err: errors.New("execution reverted: Data set not live"), want: true},
		{name: "custom not live string", err: errors.New("execution reverted: DataSetNotLive()"), want: true},
		{name: "custom not found string", err: errors.New("execution reverted: DataSetNotFound()"), want: true},
		{name: "custom not live selector", err: customError("DataSetNotLive"), want: true},
		{name: "custom not found selector", err: customError("DataSetNotFound"), want: true},
		{name: "other selector", err: customError("CleanupDepositRequired"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsDataSetUnavailable(tt.err); got != tt.want {
				t.Fatalf("IsDataSetUnavailable() = %t, want %t", got, tt.want)
			}
		})
	}
}
