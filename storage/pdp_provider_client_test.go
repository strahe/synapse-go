package storage_test

import (
	"context"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/storage"
)

var (
	_ storage.PDPProviderClient = (*pdp.Client)(nil)
	_ storage.PDPProviderClient = (*fakePDPProviderClient)(nil)
)

type fakePDPProviderClient struct{}

func (*fakePDPProviderClient) UploadPieceStreaming(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
	return nil, nil
}

func (*fakePDPProviderClient) DownloadPiece(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
	return nil, 0, nil
}

func (*fakePDPProviderClient) WaitForPieceParked(context.Context, cid.Cid, time.Duration) error {
	return nil
}

func (*fakePDPProviderClient) WaitForPullComplete(context.Context, pdp.PullRequest, time.Duration, func(*pdp.PullResult)) (*pdp.PullResult, error) {
	return nil, nil
}

func (*fakePDPProviderClient) AddPieces(context.Context, uint64, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
	return nil, nil
}

func (*fakePDPProviderClient) WaitForPiecesAdded(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
	return nil, nil
}

func (*fakePDPProviderClient) CreateDataSet(context.Context, common.Address, []byte) (*pdp.CreateDataSetResult, error) {
	return nil, nil
}

func (*fakePDPProviderClient) WaitForDataSetCreated(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error) {
	return nil, nil
}

func (*fakePDPProviderClient) CreateDataSetAndAddPieces(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
	return nil, nil
}

func (*fakePDPProviderClient) WaitForCreateDataSetAndAddPieces(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
	return nil, nil
}

func (*fakePDPProviderClient) SchedulePieceDeletion(context.Context, uint64, uint64, []byte) (common.Hash, error) {
	return common.Hash{}, nil
}
