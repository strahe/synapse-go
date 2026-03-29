package storage

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	icurio "github.com/strahe/synapse-go/internal/curio"
	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
)

var (
	contextAddressType, _       = abi.NewType("address", "", nil)
	contextUint256Type, _       = abi.NewType("uint256", "", nil)
	contextStringArrayType, _   = abi.NewType("string[]", "", nil)
	contextStringArray2DType, _ = abi.NewType("string[][]", "", nil)
	contextBytesType, _         = abi.NewType("bytes", "", nil)
)

const (
	maxMetadataKeyLength   = 32
	maxMetadataValueLength = 128
	maxDataSetMetadataKeys = 10
	maxPieceMetadataKeys   = 5
)

type PDPClient interface {
	UploadPieceFromBytes(context.Context, cid.Cid, []byte) (*icurio.UploadPieceResult, error)
	DownloadPiece(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	WaitForPieceParked(context.Context, cid.Cid, time.Duration) error
	WaitForPullComplete(context.Context, icurio.PullRequest, time.Duration, func(*icurio.PullResult)) (*icurio.PullResult, error)
	AddPieces(context.Context, uint64, []icurio.AddPieceInput, []byte) (*icurio.AddPiecesResult, error)
	WaitForPiecesAdded(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
	CreateDataSetAndAddPieces(context.Context, common.Address, []icurio.AddPieceInput, []byte) (*icurio.CreateDataSetResult, error)
	WaitForCreateDataSetAndAddPieces(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
}

type Provider struct {
	ID              *big.Int
	ServiceURL      string
	ServiceProvider common.Address
	Payee           common.Address
}

type ContextOption func(*Context)

type Context struct {
	// commitMu serialises Commit calls so the create-vs-add path decision
	// made in PresignForCommit and the subsequent curio API call are
	// always consistent under concurrent use.
	commitMu sync.Mutex
	mu       sync.RWMutex

	provider     Provider
	client       PDPClient
	signer       signer.EVMSigner
	payer        common.Address
	chainID      *big.Int
	recordKeeper common.Address
	withCDN      bool

	dataSetID       *big.Int
	clientDataSetID *big.Int
	dataSetMetadata map[string]string
}

func NewContext(provider Provider, client PDPClient, evmSigner signer.EVMSigner, opts ...ContextOption) (*Context, error) {
	if provider.ID == nil {
		return nil, errors.New("storage.NewContext: nil provider ID")
	}
	if provider.ServiceURL == "" {
		return nil, errors.New("storage.NewContext: empty provider service URL")
	}
	if client == nil {
		return nil, errors.New("storage.NewContext: nil PDP client")
	}
	c := &Context{
		provider: Provider{
			ID:              new(big.Int).Set(provider.ID),
			ServiceURL:      provider.ServiceURL,
			ServiceProvider: provider.ServiceProvider,
			Payee:           provider.Payee,
		},
		client: client,
		signer: evmSigner,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c, nil
}

func WithPayer(payer common.Address) ContextOption {
	return func(c *Context) { c.payer = payer }
}

func WithChainID(chainID *big.Int) ContextOption {
	return func(c *Context) {
		if chainID != nil {
			c.chainID = new(big.Int).Set(chainID)
		}
	}
}

func WithRecordKeeper(addr common.Address) ContextOption {
	return func(c *Context) { c.recordKeeper = addr }
}

func WithDataSetID(id *big.Int) ContextOption {
	return func(c *Context) {
		if id != nil {
			c.dataSetID = new(big.Int).Set(id)
		}
	}
}

func WithClientDataSetID(id *big.Int) ContextOption {
	return func(c *Context) {
		if id != nil {
			c.clientDataSetID = new(big.Int).Set(id)
		}
	}
}

func WithDataSetMetadata(metadata map[string]string) ContextOption {
	return func(c *Context) { c.dataSetMetadata = cloneStringMap(metadata) }
}

func WithCDN(enabled bool) ContextOption {
	return func(c *Context) { c.withCDN = enabled }
}

func (c *Context) StoreBytes(ctx context.Context, data []byte, opts *StoreOptions) (*StoreResult, error) {
	if len(data) == 0 {
		return nil, errors.New("storage.Context.StoreBytes: empty data")
	}
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.StoreBytes: calculate piece: %w", err)
	}
	if !info.CIDv2.Defined() {
		return nil, errors.New("storage.Context.StoreBytes: undefined PieceCIDv2")
	}
	if _, err := c.client.UploadPieceFromBytes(ctx, info.CIDv2, data); err != nil {
		return nil, fmt.Errorf("storage.Context.StoreBytes: upload: %w", err)
	}
	if err := c.client.WaitForPieceParked(ctx, info.CIDv2, 0); err != nil {
		return nil, fmt.Errorf("storage.Context.StoreBytes: wait for parked: %w", err)
	}
	_ = opts
	return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
}

func (c *Context) PresignForCommit(_ context.Context, pieces []PieceInput) ([]byte, error) {
	if len(pieces) == 0 {
		return nil, errors.New("storage.Context.PresignForCommit: no pieces provided")
	}
	if c.signer == nil {
		return nil, errors.New("storage.Context.PresignForCommit: nil signer")
	}
	if c.chainID == nil {
		return nil, errors.New("storage.Context.PresignForCommit: nil chainID")
	}
	if c.recordKeeper == (common.Address{}) {
		return nil, errors.New("storage.Context.PresignForCommit: zero recordKeeper")
	}
	if c.payer == (common.Address{}) {
		return nil, errors.New("storage.Context.PresignForCommit: zero payer")
	}

	pieceCIDs := make([]cid.Cid, 0, len(pieces))
	pieceMetadata := make([][]ityped.MetadataEntry, 0, len(pieces))
	for _, p := range pieces {
		if !p.PieceCID.Defined() {
			return nil, errors.New("storage.Context.PresignForCommit: undefined pieceCID")
		}
		pieceCIDs = append(pieceCIDs, p.PieceCID)
		meta, err := pieceMetadataEntries(p.PieceMetadata)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
		}
		pieceMetadata = append(pieceMetadata, meta)
	}

	domain := ityped.NewDomain(c.chainID, c.recordKeeper)

	c.mu.Lock()
	defer c.mu.Unlock()

	clientDataSetID := c.clientDataSetID
	if clientDataSetID == nil {
		clientDataSetID = randomUint256()
		c.clientDataSetID = new(big.Int).Set(clientDataSetID)
	}
	if c.dataSetID != nil {
		nonce := randomUint256()
		sig, err := ityped.SignAddPieces(c.signer.SignHash, domain, clientDataSetID, nonce, pieceCIDs, pieceMetadata)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: sign add pieces: %w", err)
		}
		return encodeAddPiecesExtraData(nonce, pieceMetadata, signatureBytes(sig))
	}

	dataSetMetadata, err := dataSetMetadataEntries(c.dataSetMetadata, c.withCDN)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}
	createSig, err := ityped.SignCreateDataSet(c.signer.SignHash, domain, clientDataSetID, c.provider.Payee, dataSetMetadata)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: sign create dataset: %w", err)
	}
	nonce := randomUint256()
	addSig, err := ityped.SignAddPieces(c.signer.SignHash, domain, clientDataSetID, nonce, pieceCIDs, pieceMetadata)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: sign add pieces: %w", err)
	}
	createPayload, err := encodeCreateDataSetExtraData(c.payer, clientDataSetID, dataSetMetadata, signatureBytes(createSig))
	if err != nil {
		return nil, err
	}
	addPayload, err := encodeAddPiecesExtraData(nonce, pieceMetadata, signatureBytes(addSig))
	if err != nil {
		return nil, err
	}
	return encodeCreateAndAddExtraData(createPayload, addPayload)
}

func (c *Context) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	if len(req.Pieces) == 0 {
		return nil, errors.New("storage.Context.Pull: no pieces provided")
	}
	if req.From == nil {
		return nil, errors.New("storage.Context.Pull: nil source resolver")
	}
	curioReq := icurio.PullRequest{
		ExtraData: append([]byte(nil), req.ExtraData...),
	}

	c.mu.RLock()
	dataSetID := copyBigInt(c.dataSetID)
	recordKeeper := c.recordKeeper
	c.mu.RUnlock()

	// RecordKeeper is required by curio for both new and existing datasets.
	curioReq.RecordKeeper = recordKeeper
	if dataSetID != nil {
		if !dataSetID.IsUint64() {
			return nil, errors.New("storage.Context.Pull: dataSetID exceeds uint64")
		}
		curioReq.DataSetID = dataSetID.Uint64()
	}

	pieceByString := make(map[string]cid.Cid, len(req.Pieces))
	for _, pieceCID := range req.Pieces {
		if !pieceCID.Defined() {
			return nil, errors.New("storage.Context.Pull: undefined pieceCID")
		}
		sourceURL := req.From(pieceCID)
		if sourceURL == "" {
			return nil, errors.New("storage.Context.Pull: empty source URL")
		}
		curioReq.Pieces = append(curioReq.Pieces, icurio.PullPieceInput{
			PieceCID:  pieceCID,
			SourceURL: sourceURL,
		})
		pieceByString[pieceCID.String()] = pieceCID
	}

	res, err := c.client.WaitForPullComplete(ctx, curioReq, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Pull: %w", err)
	}

	out := &PullResult{Status: PullStatus(res.Status)}
	for _, pieceStatus := range res.Pieces {
		out.Pieces = append(out.Pieces, PullPieceResult{
			PieceCID: pieceByString[pieceStatus.PieceCID],
			Status:   PullStatus(pieceStatus.Status),
		})
	}
	return out, nil
}

func (c *Context) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	if len(req.Pieces) == 0 {
		return nil, errors.New("storage.Context.Commit: no pieces provided")
	}

	// Serialise all Commit calls to prevent a TOCTOU race: the create-vs-add
	// decision is made in PresignForCommit (which reads c.dataSetID) and then
	// acted on below (also reading c.dataSetID).  Without serialisation, two
	// concurrent Commits can both see dataSetID==nil and both create a new
	// dataset, corrupting the on-chain state.
	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	// Snapshot create-vs-add decision under the data lock BEFORE signing.
	c.mu.RLock()
	dataSetID := copyBigInt(c.dataSetID)
	recordKeeper := c.recordKeeper
	c.mu.RUnlock()

	extraData := append([]byte(nil), req.ExtraData...)
	var err error
	if len(extraData) == 0 {
		extraData, err = c.PresignForCommit(ctx, req.Pieces)
		if err != nil {
			return nil, err
		}
		// PresignForCommit also reads c.dataSetID under c.mu. Because commitMu
		// prevents concurrent Commits, the snapshot above matches what
		// PresignForCommit saw, so create-vs-add is consistent.
	}

	pieces := make([]icurio.AddPieceInput, 0, len(req.Pieces))
	for _, p := range req.Pieces {
		pieces = append(pieces, icurio.AddPieceInput{PieceCID: p.PieceCID})
	}

	if dataSetID != nil {
		if !dataSetID.IsUint64() {
			return nil, errors.New("storage.Context.Commit: dataSetID exceeds uint64")
		}
		added, err := c.client.AddPieces(ctx, dataSetID.Uint64(), pieces, extraData)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.Commit: add pieces: %w", err)
		}
		status, err := c.client.WaitForPiecesAdded(ctx, added.StatusURL, 0)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.Commit: wait add pieces: %w", err)
		}
		return &CommitResult{
			TransactionID: status.TxHash.Hex(),
			DataSetID:     new(big.Int).SetUint64(status.DataSetID),
			PieceIDs:      cloneBigInts(status.ConfirmedPieceIDs),
			IsNewDataSet:  false,
		}, nil
	}

	created, err := c.client.CreateDataSetAndAddPieces(ctx, recordKeeper, pieces, extraData)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Commit: create and add pieces: %w", err)
	}
	// Note: if WaitForCreateDataSetAndAddPieces fails here (e.g. timeout) after
	// the transaction was already submitted on-chain, c.dataSetID will not be
	// set. A subsequent retry will call CreateDataSetAndAddPieces again with
	// the same clientDataSetID; idempotency depends on the contract rejecting
	// duplicate clientDataSetIDs.
	status, err := c.client.WaitForCreateDataSetAndAddPieces(ctx, created.StatusURL, 0)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Commit: wait create and add pieces: %w", err)
	}
	if status.DataSetID == 0 {
		return nil, errors.New("storage.Context.Commit: server returned zero dataSetID")
	}
	result := &CommitResult{
		TransactionID: status.TxHash.Hex(),
		DataSetID:     new(big.Int).SetUint64(status.DataSetID),
		PieceIDs:      cloneBigInts(status.ConfirmedPieceIDs),
		IsNewDataSet:  true,
	}
	c.mu.Lock()
	c.dataSetID = copyBigInt(result.DataSetID)
	c.mu.Unlock()
	return result, nil
}

func (c *Context) PieceURL(pieceCID cid.Cid) string {
	return c.pieceURLFor(pieceCID)
}

func (c *Context) ProviderID() *big.Int {
	return copyBigInt(c.provider.ID)
}

func (c *Context) ServiceURL() string {
	return c.provider.ServiceURL
}

func (c *Context) pieceURLFor(pieceCID cid.Cid) string {
	base, err := url.Parse(c.provider.ServiceURL)
	if err != nil {
		return c.provider.ServiceURL
	}
	base.Path = path.Join(base.Path, "piece", pieceCID.String())
	return base.String()
}

func dataSetMetadataEntries(metadata map[string]string, withCDN bool) ([]ityped.MetadataEntry, error) {
	merged := cloneStringMap(metadata)
	if withCDN {
		if merged == nil {
			merged = map[string]string{}
		}
		merged["withCDN"] = ""
	}
	return metadataEntries(merged, maxDataSetMetadataKeys)
}

func pieceMetadataEntries(metadata map[string]string) ([]ityped.MetadataEntry, error) {
	return metadataEntries(metadata, maxPieceMetadataKeys)
}

func metadataEntries(metadata map[string]string, maxKeys int) ([]ityped.MetadataEntry, error) {
	if len(metadata) > maxKeys {
		return nil, fmt.Errorf("storage: metadata exceeds maximum key count %d", maxKeys)
	}
	keys := make([]string, 0, len(metadata))
	for k, v := range metadata {
		if len(k) > maxMetadataKeyLength {
			return nil, fmt.Errorf("storage: metadata key %q exceeds max length %d", k, maxMetadataKeyLength)
		}
		if len(v) > maxMetadataValueLength {
			return nil, fmt.Errorf("storage: metadata value for %q exceeds max length %d", k, maxMetadataValueLength)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ityped.MetadataEntry, 0, len(keys))
	for _, k := range keys {
		out = append(out, ityped.MetadataEntry{Key: k, Value: metadata[k]})
	}
	return out, nil
}

func encodeCreateDataSetExtraData(payer common.Address, clientDataSetID *big.Int, metadata []ityped.MetadataEntry, signature []byte) ([]byte, error) {
	keys := make([]string, 0, len(metadata))
	values := make([]string, 0, len(metadata))
	for _, m := range metadata {
		keys = append(keys, m.Key)
		values = append(values, m.Value)
	}
	args := abi.Arguments{
		{Type: contextAddressType},
		{Type: contextUint256Type},
		{Type: contextStringArrayType},
		{Type: contextStringArrayType},
		{Type: contextBytesType},
	}
	out, err := args.Pack(payer, clientDataSetID, keys, values, signature)
	if err != nil {
		return nil, fmt.Errorf("storage: encode create dataset extraData: %w", err)
	}
	return out, nil
}

func encodeAddPiecesExtraData(nonce *big.Int, metadata [][]ityped.MetadataEntry, signature []byte) ([]byte, error) {
	keys := make([][]string, len(metadata))
	values := make([][]string, len(metadata))
	for i, pieceMetadata := range metadata {
		keys[i] = make([]string, len(pieceMetadata))
		values[i] = make([]string, len(pieceMetadata))
		for j, m := range pieceMetadata {
			keys[i][j] = m.Key
			values[i][j] = m.Value
		}
	}
	args := abi.Arguments{
		{Type: contextUint256Type},
		{Type: contextStringArray2DType},
		{Type: contextStringArray2DType},
		{Type: contextBytesType},
	}
	out, err := args.Pack(nonce, keys, values, signature)
	if err != nil {
		return nil, fmt.Errorf("storage: encode add pieces extraData: %w", err)
	}
	return out, nil
}

func encodeCreateAndAddExtraData(createPayload, addPayload []byte) ([]byte, error) {
	args := abi.Arguments{{Type: contextBytesType}, {Type: contextBytesType}}
	out, err := args.Pack(createPayload, addPayload)
	if err != nil {
		return nil, fmt.Errorf("storage: encode create+add extraData: %w", err)
	}
	return out, nil
}

func signatureBytes(sig *ityped.Signature) []byte {
	if sig == nil {
		return nil
	}
	out := make([]byte, 65)
	copy(out[:32], sig.R[:])
	copy(out[32:64], sig.S[:])
	out[64] = sig.V
	return out
}

func randomUint256() *big.Int {
	var buf [32]byte
	_, _ = rand.Read(buf[:])
	return new(big.Int).SetBytes(buf[:])
}

func cloneBigInts(in []*big.Int) []*big.Int {
	out := make([]*big.Int, 0, len(in))
	for _, v := range in {
		out = append(out, copyBigInt(v))
	}
	return out
}

func copyBigInt(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
