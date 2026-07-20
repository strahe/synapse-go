package warmstorage

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/txutil"
	sdktypes "github.com/strahe/synapse-go/types"
)

func TestTopUpCDNPaymentRails_BroadcastsExpectedCall(t *testing.T) {
	svc, backend := newWriteTestService(t)
	res, err := svc.TopUpCDNPaymentRails(
		context.Background(),
		sdktypes.NewBigInt(7),
		big.NewInt(11),
		big.NewInt(13),
	)
	if err != nil {
		t.Fatalf("TopUpCDNPaymentRails: %v", err)
	}
	if len(backend.sent) != 1 || res == nil || res.Hash != backend.sent[0].Hash() {
		t.Fatalf("result=%+v sent=%d", res, len(backend.sent))
	}
	method := backend.fwssABI.Methods["topUpCDNPaymentRails"]
	args, err := method.Inputs.Unpack(backend.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack calldata: %v", err)
	}
	if args[0].(*big.Int).Cmp(big.NewInt(7)) != 0 ||
		args[1].(*big.Int).Cmp(big.NewInt(11)) != 0 ||
		args[2].(*big.Int).Cmp(big.NewInt(13)) != 0 {
		t.Fatalf("TopUpCDNPaymentRails args = %v", args)
	}
}

func TestTopUpCDNPaymentRails_ValidatesArguments(t *testing.T) {
	tests := []struct {
		name      string
		dataSetID sdktypes.BigInt
		cdn       *big.Int
		cacheMiss *big.Int
	}{
		{name: "zero data set", dataSetID: sdktypes.NewBigInt(0), cdn: big.NewInt(1), cacheMiss: big.NewInt(0)},
		{name: "nil CDN amount", dataSetID: sdktypes.NewBigInt(1), cacheMiss: big.NewInt(1)},
		{name: "negative CDN amount", dataSetID: sdktypes.NewBigInt(1), cdn: big.NewInt(-1), cacheMiss: big.NewInt(1)},
		{name: "nil cache miss amount", dataSetID: sdktypes.NewBigInt(1), cdn: big.NewInt(1)},
		{name: "negative cache miss amount", dataSetID: sdktypes.NewBigInt(1), cdn: big.NewInt(1), cacheMiss: big.NewInt(-1)},
		{name: "both zero", dataSetID: sdktypes.NewBigInt(1), cdn: big.NewInt(0), cacheMiss: big.NewInt(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, backend := newWriteTestService(t)
			if _, err := svc.TopUpCDNPaymentRails(context.Background(), tt.dataSetID, tt.cdn, tt.cacheMiss); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
			if len(backend.sent) != 0 {
				t.Fatalf("sent tx count = %d, want 0", len(backend.sent))
			}
		})
	}
}

func TestWriteMethods_UseBackendPendingNonce(t *testing.T) {
	svc, backend := newWriteTestService(t)
	backend.nonces[svc.signer.EVMAddress()] = 41
	if _, err := svc.TopUpCDNPaymentRails(context.Background(), sdktypes.NewBigInt(1), big.NewInt(1), big.NewInt(0)); err != nil {
		t.Fatalf("TopUpCDNPaymentRails: %v", err)
	}
	if len(backend.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(backend.sent))
	}
	if backend.sent[0].Nonce() != 41 {
		t.Fatalf("sent nonce = %d, want 41", backend.sent[0].Nonce())
	}
}

func TestTerminateDataSet_BroadcastsAndWaits(t *testing.T) {
	svc, backend := newWriteTestService(t)
	backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
		return &coretypes.Receipt{
			Status:      coretypes.ReceiptStatusSuccessful,
			TxHash:      hash,
			BlockNumber: big.NewInt(5),
		}, nil
	}

	res, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(23), WithWait(time.Second))
	if err != nil {
		t.Fatalf("TerminateDataSet: %v", err)
	}
	if res == nil || res.Receipt == nil || res.Receipt.Status != coretypes.ReceiptStatusSuccessful {
		t.Fatalf("TerminateDataSet result = %+v", res)
	}
	if len(backend.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(backend.sent))
	}
	method := backend.fwssABI.Methods["terminateService"]
	args, err := method.Inputs.Unpack(backend.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack calldata: %v", err)
	}
	if got := args[0].(*big.Int); got.Cmp(big.NewInt(23)) != 0 {
		t.Fatalf("dataSetID = %s, want 23", got)
	}
}

func TestTerminateDataSet_ValidatesDataSetAndChain(t *testing.T) {
	svc, backend := newWriteTestService(t)
	if _, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(0)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero data set error = %v, want ErrInvalidArgument", err)
	}
	svc.chainID = 0
	if _, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(1)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid chain error = %v, want ErrInvalidArgument", err)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(backend.sent))
	}
}

func TestTerminateDataSet_PropagatesSetupAndBroadcastErrors(t *testing.T) {
	t.Run("uninitialized service", func(t *testing.T) {
		var svc Service
		if _, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(1)); !errors.Is(err, ErrUninitialized) {
			t.Fatalf("error = %v, want ErrUninitialized", err)
		}
	})

	t.Run("nonce", func(t *testing.T) {
		svc, backend := newWriteTestService(t)
		want := errors.New("nonce unavailable")
		backend.nonceErr = want
		if _, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(1)); !errors.Is(err, want) {
			t.Fatalf("error = %v, want wrapped %v", err, want)
		}
	})

	t.Run("broadcast", func(t *testing.T) {
		svc, backend := newWriteTestService(t)
		want := errors.New("broadcast rejected")
		backend.sendErr = want
		if _, err := svc.TerminateDataSet(context.Background(), sdktypes.NewBigInt(1)); !errors.Is(err, want) {
			t.Fatalf("error = %v, want wrapped %v", err, want)
		}
	})
}

func TestFinalize_WithConfirmationsAndFailedReceipt(t *testing.T) {
	t.Run("confirmations", func(t *testing.T) {
		svc, backend := newWriteTestService(t)
		backend.blockNumber = 7
		backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
			return &coretypes.Receipt{
				Status:      coretypes.ReceiptStatusSuccessful,
				TxHash:      hash,
				BlockNumber: big.NewInt(5),
			}, nil
		}
		tx := coretypes.NewTx(&coretypes.LegacyTx{Nonce: 1, To: &svc.fwssAddr})
		res, err := svc.finalize(context.Background(), tx, []WriteOption{WithWait(time.Second), WithConfirmations(3)})
		if err != nil || res == nil || res.Receipt == nil {
			t.Fatalf("finalize = %+v, %v", res, err)
		}
	})

	t.Run("failed receipt", func(t *testing.T) {
		svc, backend := newWriteTestService(t)
		backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
			return &coretypes.Receipt{Status: coretypes.ReceiptStatusFailed, TxHash: hash, BlockNumber: big.NewInt(5)}, nil
		}
		tx := coretypes.NewTx(&coretypes.LegacyTx{Nonce: 2, To: &svc.fwssAddr})
		res, err := svc.finalize(context.Background(), tx, []WriteOption{WithWait(time.Second)})
		if !errors.Is(err, txutil.ErrTxFailed) {
			t.Fatalf("error = %v, want ErrTxFailed", err)
		}
		if res == nil || res.Receipt == nil || res.Receipt.Status != coretypes.ReceiptStatusFailed {
			t.Fatalf("failed result = %+v", res)
		}
	})
}

func TestExtractPDPPaymentTerminatedEvent(t *testing.T) {
	contractABI, err := fwssbind.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	event := contractABI.Events["PDPPaymentTerminated"]
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(99), big.NewInt(31))
	if err != nil {
		t.Fatal(err)
	}
	receipt := &coretypes.Receipt{Logs: []*coretypes.Log{
		nil,
		{Topics: []common.Hash{event.ID, common.BigToHash(big.NewInt(17))}, Data: data},
	}}

	got, err := ExtractPDPPaymentTerminatedEvent(receipt)
	if err != nil {
		t.Fatalf("ExtractPDPPaymentTerminatedEvent: %v", err)
	}
	if !got.DataSetID.Equal(sdktypes.NewBigInt(17)) || got.EndEpoch != 99 || !got.PDPRailID.Equal(sdktypes.NewBigInt(31)) {
		t.Fatalf("event = %+v", got)
	}
	if _, err := ExtractPDPPaymentTerminatedEvent(nil); err == nil || !strings.Contains(err.Error(), "nil receipt") {
		t.Fatalf("nil receipt error = %v", err)
	}
	if _, err := ExtractPDPPaymentTerminatedEvent(&coretypes.Receipt{}); err == nil || !strings.Contains(err.Error(), "event not found") {
		t.Fatalf("missing event error = %v", err)
	}
	malformed := &coretypes.Receipt{Logs: []*coretypes.Log{{Topics: []common.Hash{event.ID}}}}
	if _, err := ExtractPDPPaymentTerminatedEvent(malformed); err == nil || !strings.Contains(err.Error(), "event not found") {
		t.Fatalf("malformed event error = %v", err)
	}
	overflowData, err := event.Inputs.NonIndexed().Pack(new(big.Int).Lsh(big.NewInt(1), 65), big.NewInt(31))
	if err != nil {
		t.Fatal(err)
	}
	overflow := &coretypes.Receipt{Logs: []*coretypes.Log{{
		Topics: []common.Hash{event.ID, common.BigToHash(big.NewInt(17))},
		Data:   overflowData,
	}}}
	if _, err := ExtractPDPPaymentTerminatedEvent(overflow); err == nil || !strings.Contains(err.Error(), "EndEpoch") {
		t.Fatalf("overflow end epoch error = %v", err)
	}
}

func TestDataSetErrors_IncludeIdentifiers(t *testing.T) {
	var nilLive *DataSetNotLiveError
	if got := nilLive.Error(); got != "<nil>" {
		t.Fatalf("nil DataSetNotLiveError = %q", got)
	}
	var nilManaged *DataSetNotManagedError
	if got := nilManaged.Error(); got != "<nil>" {
		t.Fatalf("nil DataSetNotManagedError = %q", got)
	}
	live := (&DataSetNotLiveError{DataSetID: sdktypes.NewBigInt(5)}).Error()
	if !strings.Contains(live, "data set 5") {
		t.Fatalf("DataSetNotLiveError = %q", live)
	}
	managed := (&DataSetNotManagedError{
		DataSetID:        sdktypes.NewBigInt(6),
		Listener:         common.HexToAddress("0x6001"),
		ExpectedListener: common.HexToAddress("0x6002"),
	}).Error()
	if !strings.Contains(managed, "data set 6") || !strings.Contains(managed, "0x0000000000000000000000000000000000006001") {
		t.Fatalf("DataSetNotManagedError = %q", managed)
	}
}

func TestEpochFromBig_RejectsInvalidContractValues(t *testing.T) {
	tests := []struct {
		name  string
		value *big.Int
	}{
		{name: "nil"},
		{name: "negative", value: big.NewInt(-1)},
		{name: "overflow", value: new(big.Int).Lsh(big.NewInt(1), 65)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := epochFromBig("epoch", tt.value); err == nil {
				t.Fatal("epochFromBig returned nil error")
			}
		})
	}
}
