package storage

import (
	"context"
	"testing"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSDataSetReader struct {
	calls   int
	gotID   types.BigInt
	info    *warmstorage.DataSetInfo
	infoErr error
}

func (f *fakeFWSSDataSetReader) GetDataSet(_ context.Context, id types.BigInt) (*warmstorage.DataSetInfo, error) {
	f.calls++
	f.gotID = id
	return f.info, f.infoErr
}

func TestValidateUploadContextsWritable_DoesNotMutateImmutableTarget(t *testing.T) {
	dataSetID := types.NewBigInt(99)
	clientDataSetID := types.NewBigInt(0xBEEF)
	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{
			DataSetID:       dataSetID,
			ProviderID:      testProvider().ID,
			ClientDataSetID: clientDataSetID,
		},
	}
	dataSetCtx, err := NewDataSetContext(
		testProvider(),
		&fakePDPProviderClient{},
		mustTestSigner(t),
		testDataSetRef(dataSetID, clientDataSetID),
	)
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}

	svc := newTestService()
	svc.dsReader = reader
	if err := svc.validateUploadContextsWritable(context.Background(), []StorageContext{dataSetCtx}); err != nil {
		t.Fatalf("validateUploadContextsWritable: %v", err)
	}
	if reader.calls != 1 || !reader.gotID.Equal(dataSetID) {
		t.Fatalf("reader calls=%d dataSetID=%s", reader.calls, reader.gotID.String())
	}
	ref, ok := dataSetCtx.DataSetRef()
	if !ok || !ref.ClientDataSetID().Equal(clientDataSetID) {
		t.Fatalf("DataSetRef()=(%+v, %t), want immutable clientDataSetID %s", ref, ok, clientDataSetID.String())
	}
}
