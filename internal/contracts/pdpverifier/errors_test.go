package pdpverifier

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type revertDataError struct {
	data string
	msg  string
}

func (e revertDataError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return "execution reverted"
}

func (e revertDataError) ErrorCode() int {
	return 3
}

func (e revertDataError) ErrorData() any {
	return e.data
}

func customError(name string) error {
	return revertDataError{data: customErrorData(name)}
}

func customErrorData(name string) string {
	hash := crypto.Keccak256([]byte(name + "()"))
	return hexutil.Encode(hash[:4])
}

func revertReasonData(t *testing.T, reason string) []byte {
	t.Helper()
	stringType, err := abi.NewType("string", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := (abi.Arguments{{Type: stringType}}).Pack(reason)
	if err != nil {
		t.Fatal(err)
	}
	return append(hexutil.MustDecode("0x08c379a0"), encoded...)
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
		{name: "near match", err: errors.New("execution reverted: DataSetNotLiveSoon()"), want: false},
		{name: "transport text suffix", err: errors.New("transport failed: DataSetNotLive()"), want: false},
		{
			name: "unknown selector does not trust surrounding text",
			err: revertDataError{
				data: customErrorData("CleanupDepositRequired"),
				msg:  "execution reverted: DataSetNotLive()",
			},
			want: false,
		},
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

func TestIsDataSetUnavailableData(t *testing.T) {
	t.Parallel()

	if !IsDataSetUnavailableData(revertReasonData(t, "Data set not live")) {
		t.Fatal("legacy Error(string) reason was not classified")
	}
	if IsDataSetUnavailableData(revertReasonData(t, "Data set not live soon")) {
		t.Fatal("near-match Error(string) reason was classified")
	}
	if IsDataSetUnavailableData([]byte{0xde, 0xad, 0xbe, 0xef}) {
		t.Fatal("unknown custom error was classified")
	}
}
