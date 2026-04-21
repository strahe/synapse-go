package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	icurio "github.com/strahe/synapse-go/internal/curio"
	"github.com/strahe/synapse-go/internal/idconv"
	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

var (
	contextAddressType, _       = abi.NewType("address", "", nil)
	contextUint256Type, _       = abi.NewType("uint256", "", nil)
	contextStringArrayType, _   = abi.NewType("string[]", "", nil)
	contextStringArray2DType, _ = abi.NewType("string[][]", "", nil)
	contextBytesType, _         = abi.NewType("bytes", "", nil)
)

var randReader io.Reader = rand.Reader

const (
	maxMetadataKeyLength   = 32
	maxMetadataValueLength = 128
	maxDataSetMetadataKeys = 10
	maxPieceMetadataKeys   = 5
)

// PDPClient is the curio HTTP API surface required by Context.
// Satisfied by *internal/curio.Client; injectable for testing.
type PDPClient interface {
	UploadPieceStreaming(context.Context, io.Reader, icurio.UploadPieceStreamingOptions) (*icurio.UploadStreamingResult, error)
	DownloadPiece(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	WaitForPieceParked(context.Context, cid.Cid, time.Duration) error
	WaitForPullComplete(context.Context, icurio.PullRequest, time.Duration, func(*icurio.PullResult)) (*icurio.PullResult, error)
	AddPieces(context.Context, uint64, []icurio.AddPieceInput, []byte) (*icurio.AddPiecesResult, error)
	WaitForPiecesAdded(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
	CreateDataSetAndAddPieces(context.Context, common.Address, []icurio.AddPieceInput, []byte) (*icurio.CreateDataSetResult, error)
	WaitForCreateDataSetAndAddPieces(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
}

// Provider holds the on-chain identity of a storage provider.
type Provider struct {
	ID              types.ProviderID // numeric provider ID from SPRegistry
	ServiceURL      string           // base URL of the provider's curio HTTP API
	ServiceProvider common.Address   // provider's EVM address
	Payee           common.Address   // address that receives payments
}

// ContextOption configures a Context.
type ContextOption func(*Context)

// Context represents a specific provider + data-set pair and handles
// storage operations (store, pull, and/or commit) for one upload copy.
// It is safe for concurrent use.
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

	dataSetID       *types.DataSetID
	clientDataSetID types.ClientDataSetID
	dataSetMetadata map[string]string
	presignedKinds  map[[32]byte]commitExtraDataKind
}

type commitExtraDataKind uint8

const (
	commitExtraDataUnknown commitExtraDataKind = iota
	commitExtraDataAddOnly
	commitExtraDataCreateAndAdd
)

// NewContext creates a Context for the given provider and PDP client.
// provider.ID, provider.ServiceURL, and client are validated here. Signing
// prerequisites (such as a non-nil signer plus chain/payer/record-keeper
// options) are validated by the write paths that need them, e.g.
// PresignForCommit.
func NewContext(provider Provider, client PDPClient, evmSigner signer.EVMSigner, opts ...ContextOption) (*Context, error) {
	if provider.ID == 0 {
		return nil, fmt.Errorf("storage.NewContext: %w: zero provider ID", ErrInvalidArgument)
	}
	if provider.ServiceURL == "" {
		return nil, fmt.Errorf("storage.NewContext: %w: empty provider service URL", ErrInvalidArgument)
	}
	if client == nil {
		return nil, fmt.Errorf("storage.NewContext: %w: nil PDP client", ErrInvalidArgument)
	}
	c := &Context{
		provider: Provider{
			ID:              provider.ID,
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
	if c.dataSetID != nil && *c.dataSetID == 0 {
		return nil, fmt.Errorf("storage.NewContext: %w: zero dataSetID", ErrInvalidArgument)
	}
	return c, nil
}

// WithPayer sets the EVM address that pays for storage.
func WithPayer(payer common.Address) ContextOption {
	return func(c *Context) { c.payer = payer }
}

// WithChainID sets the EIP-155 chain ID used for EIP-712 domain separation.
func WithChainID(chainID *big.Int) ContextOption {
	return func(c *Context) {
		if chainID != nil {
			c.chainID = new(big.Int).Set(chainID)
		}
	}
}

// WithRecordKeeper sets the FWSS contract address (record-keeper) used for
// EIP-712 signing and passed to Curio for Pull and CreateDataSet operations.
func WithRecordKeeper(addr common.Address) ContextOption {
	return func(c *Context) { c.recordKeeper = addr }
}

// WithDataSetID pins the context to an existing on-chain data set.
// When set, Commit issues an AddPieces call instead of CreateDataSet+AddPieces.
func WithDataSetID(id types.DataSetID) ContextOption {
	return func(c *Context) {
		v := id
		c.dataSetID = &v
	}
}

// WithClientDataSetID sets a caller-chosen data-set identifier included in
// EIP-712 messages. If not provided, a random value is generated on the
// first PresignForCommit call and reused for the lifetime of this Context.
func WithClientDataSetID(id types.ClientDataSetID) ContextOption {
	return func(c *Context) {
		if id == nil {
			c.clientDataSetID = nil
			return
		}
		c.clientDataSetID = new(big.Int).Set(id)
	}
}

// WithDataSetMetadata sets the key-value metadata stored with the data set on creation.
func WithDataSetMetadata(metadata map[string]string) ContextOption {
	return func(c *Context) { c.dataSetMetadata = cloneStringMap(metadata) }
}

// WithCDN enables CDN services for the data set. When true, a "withCDN"
// metadata marker is added to the EIP-712 dataset-creation message;
// the contract activates CDN and applies its configured lockup upon seeing it.
func WithCDN(enabled bool) ContextOption {
	return func(c *Context) { c.withCDN = enabled }
}

// Store streams data to the provider and waits for it to be parked.
// The reader is consumed in a single pass. If opts.PieceCID is defined,
// the client skips inline commP calculation; otherwise commP is computed
// during the upload via TeeReader. opts may be nil.
func (c *Context) Store(ctx context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
	if r == nil {
		return nil, fmt.Errorf("storage.Context.Store: %w: nil reader", ErrInvalidArgument)
	}
	if opts == nil {
		opts = &StoreOptions{}
	}
	if opts.PieceCID.Defined() {
		if _, err := piece.ParseV2(opts.PieceCID); err != nil {
			return nil, fmt.Errorf("storage.Context.Store: invalid PieceCID: %w", err)
		}
	}
	size := detectSize(r, opts.PieceCID)
	res, err := c.client.UploadPieceStreaming(ctx, r, icurio.UploadPieceStreamingOptions{
		Size:       size,
		PieceCID:   opts.PieceCID,
		OnProgress: opts.OnProgress,
	})
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Store: upload: %w", err)
	}
	if !res.PieceCID.Defined() {
		return nil, errors.New("storage.Context.Store: upload returned undefined PieceCIDv2")
	}
	if err := c.client.WaitForPieceParked(ctx, res.PieceCID, 0); err != nil {
		return nil, fmt.Errorf("storage.Context.Store: wait for parked: %w", err)
	}
	return &StoreResult{PieceCID: res.PieceCID, Size: res.Size}, nil
}

// detectSize reports the payload size without consuming the reader when
// possible. A return value of 0 means "unknown" — callers should fall
// back to chunked transfer-encoding.
//
// Detection, in order of preference:
//  1. pc is defined → decode RawSize from the PieceCIDv2 (most accurate).
//  2. Reader type is a well-known in-memory buffer (bytes.Reader,
//     bytes.Buffer, strings.Reader) → use Len().
//  3. Reader is an *os.File referring to a regular file → Stat().Size()
//     minus the current seek position (remaining bytes).
//
// This function is intentionally side-effect free except for the
// *os.File case, which uses Seek(0, io.SeekCurrent) — a no-movement
// seek that returns the current position without advancing it.
func detectSize(r io.Reader, pc cid.Cid) int64 {
	if pc.Defined() {
		if info, err := piece.ParseV2(pc); err == nil && info.RawSize > 0 {
			if info.RawSize <= math.MaxInt64 {
				return int64(info.RawSize)
			}
		}
	}
	switch v := r.(type) {
	case *bytes.Reader:
		return int64(v.Len())
	case *bytes.Buffer:
		return int64(v.Len())
	case *strings.Reader:
		return int64(v.Len())
	case *os.File:
		if fi, err := v.Stat(); err == nil && fi.Mode().IsRegular() {
			cur, err := v.Seek(0, io.SeekCurrent)
			if err == nil && cur >= 0 && cur <= fi.Size() {
				return fi.Size() - cur
			}
		}
	}
	return 0
}

// PresignForCommit produces the EIP-712–signed extraData payload for Commit.
// For a new data set it signs both CreateDataSet and AddPieces; for an existing
// data set it signs only AddPieces. The returned bytes are opaque to callers.
//
// The operation is CPU/crypto-bound and performs no I/O, but ctx is honoured
// before each signing step so callers can cancel long batches.
func (c *Context) PresignForCommit(ctx context.Context, pieces []PieceInput) ([]byte, error) {
	if len(pieces) == 0 {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: no pieces provided", ErrInvalidArgument)
	}
	if c.signer == nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: nil signer", ErrInvalidArgument)
	}
	if c.chainID == nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: nil chainID", ErrInvalidArgument)
	}
	if c.recordKeeper == (common.Address{}) {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: zero recordKeeper", ErrInvalidArgument)
	}
	if c.payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: zero payer", ErrInvalidArgument)
	}

	pieceCIDs := make([]cid.Cid, 0, len(pieces))
	pieceMetadata := make([][]ityped.MetadataEntry, 0, len(pieces))
	for _, p := range pieces {
		if !p.PieceCID.Defined() {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w: undefined pieceCID", ErrInvalidArgument)
		}
		pieceCIDs = append(pieceCIDs, p.PieceCID)
		meta, err := pieceMetadataEntries(p.PieceMetadata)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
		}
		pieceMetadata = append(pieceMetadata, meta)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}

	domain := ityped.NewDomain(c.chainID, c.recordKeeper)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.clientDataSetID == nil {
		v, err := randomClientDataSetID()
		if err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
		}
		c.clientDataSetID = v
	}
	clientDataSetID := new(big.Int).Set(c.clientDataSetID)
	if c.dataSetID != nil {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
		}
		nonce, err := randomUint256()
		if err != nil {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
		}
		sig, err := ityped.SignAddPieces(c.signHashFunc(), domain, clientDataSetID, nonce, pieceCIDs, pieceMetadata)
		if err != nil {
			if errors.Is(err, signer.ErrUnsupportedSigner) {
				return nil, fmt.Errorf("storage.Context.PresignForCommit: wrapped/decorated EVMSigner values are unsupported: %w", err)
			}
			return nil, fmt.Errorf("storage.Context.PresignForCommit: sign add pieces: %w", err)
		}
		payload, err := encodeAddPiecesExtraData(nonce, pieceMetadata, signatureBytes(sig))
		if err != nil {
			return nil, err
		}
		return payload, nil
	}

	dataSetMetadata, err := dataSetMetadataEntries(c.dataSetMetadata, c.withCDN)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}
	createSig, err := ityped.SignCreateDataSet(c.signHashFunc(), domain, clientDataSetID, c.provider.Payee, dataSetMetadata)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: wrapped/decorated EVMSigner values are unsupported: %w", err)
		}
		return nil, fmt.Errorf("storage.Context.PresignForCommit: sign create dataset: %w", err)
	}
	nonce, err := randomUint256()
	if err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("storage.Context.PresignForCommit: %w", err)
	}
	addSig, err := ityped.SignAddPieces(c.signHashFunc(), domain, clientDataSetID, nonce, pieceCIDs, pieceMetadata)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("storage.Context.PresignForCommit: wrapped/decorated EVMSigner values are unsupported: %w", err)
		}
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
	payload, err := encodeCreateAndAddExtraData(createPayload, addPayload)
	if err != nil {
		return nil, err
	}
	c.rememberPresignedExtraDataLocked(payload, commitExtraDataCreateAndAdd)
	return payload, nil
}

// Pull asks this provider to fetch pieces from another provider (SP-to-SP transfer).
// req.ExtraData must be the payload returned by PresignForCommit on this context.
func (c *Context) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	if len(req.Pieces) == 0 {
		return nil, fmt.Errorf("storage.Context.Pull: %w: no pieces provided", ErrInvalidArgument)
	}
	if req.From == nil {
		return nil, fmt.Errorf("storage.Context.Pull: %w: nil source resolver", ErrInvalidArgument)
	}
	curioReq := icurio.PullRequest{
		ExtraData: append([]byte(nil), req.ExtraData...),
	}

	c.mu.RLock()
	dataSetID := c.dataSetID
	recordKeeper := c.recordKeeper
	c.mu.RUnlock()

	// RecordKeeper is required by curio for both new and existing datasets.
	curioReq.RecordKeeper = recordKeeper
	if dataSetID != nil {
		curioReq.DataSetID = uint64(*dataSetID)
	}

	pieceByString := make(map[string]cid.Cid, len(req.Pieces))
	for _, pieceCID := range req.Pieces {
		if !pieceCID.Defined() {
			return nil, fmt.Errorf("storage.Context.Pull: %w: undefined pieceCID", ErrInvalidArgument)
		}
		sourceURL := req.From(pieceCID)
		if sourceURL == "" {
			return nil, fmt.Errorf("storage.Context.Pull: %w: empty source URL", ErrInvalidArgument)
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

// Commit calls the provider's AddPieces or CreateDataSet+AddPieces API and
// waits for on-chain confirmation. When req.ExtraData is empty, PresignForCommit
// is called internally to produce the signed payload.
func (c *Context) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	if len(req.Pieces) == 0 {
		return nil, fmt.Errorf("storage.Context.Commit: %w: no pieces provided", ErrInvalidArgument)
	}

	// Serialise all Commit calls to prevent a TOCTOU race: the create-vs-add
	// decision is made in PresignForCommit (which reads c.dataSetID) and then
	// acted on below (also reading c.dataSetID).  Without serialisation, two
	// concurrent Commits can both see dataSetID==nil and both create a new
	// dataset, corrupting the on-chain state.
	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	extraData := append([]byte(nil), req.ExtraData...)
	var err error
	if c.presignedExtraDataIsStale(extraData) {
		c.forgetPresignedExtraData(extraData)
		extraData = nil
	}
	if len(extraData) == 0 {
		extraData, err = c.PresignForCommit(ctx, req.Pieces)
		if err != nil {
			return nil, err
		}
	}

	// Snapshot create-vs-add decision under the data lock after any required
	// re-signing so the chosen curio API matches the payload we are sending.
	c.mu.RLock()
	dataSetID := c.dataSetID
	recordKeeper := c.recordKeeper
	c.mu.RUnlock()

	pieces := make([]icurio.AddPieceInput, 0, len(req.Pieces))
	for _, p := range req.Pieces {
		pieces = append(pieces, icurio.AddPieceInput{PieceCID: p.PieceCID})
	}

	if dataSetID != nil {
		added, err := c.client.AddPieces(ctx, uint64(*dataSetID), pieces, extraData)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.Commit: add pieces: %w", err)
		}
		status, err := c.client.WaitForPiecesAdded(ctx, added.StatusURL, 0)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.Commit: wait add pieces: %w", err)
		}
		if status.DataSetID == 0 {
			return nil, errors.New("storage.Context.Commit: server returned zero dataSetID")
		}
		if got := types.DataSetID(status.DataSetID); got != *dataSetID {
			return nil, fmt.Errorf("storage.Context.Commit: server returned mismatched dataSetID: got %d want %d", got, *dataSetID)
		}
		pieceIDs, err := idconv.SafeSlice[types.PieceID]("pieceID", status.ConfirmedPieceIDs)
		if err != nil {
			return nil, fmt.Errorf("storage.Context.Commit: %w", err)
		}
		return &CommitResult{
			TransactionID: status.TxHash.Hex(),
			DataSetID:     types.DataSetID(status.DataSetID),
			PieceIDs:      pieceIDs,
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
	pieceIDs, err := idconv.SafeSlice[types.PieceID]("pieceID", status.ConfirmedPieceIDs)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Commit: %w", err)
	}
	result := &CommitResult{
		TransactionID: status.TxHash.Hex(),
		DataSetID:     types.DataSetID(status.DataSetID),
		PieceIDs:      pieceIDs,
		IsNewDataSet:  true,
	}
	c.forgetPresignedExtraData(extraData)
	newID := result.DataSetID
	c.mu.Lock()
	c.dataSetID = &newID
	c.mu.Unlock()
	return result, nil
}

// PieceURL returns the HTTPS retrieval URL for the given piece CID on this provider.
func (c *Context) PieceURL(pieceCID cid.Cid) string {
	return c.pieceURLFor(pieceCID)
}

// ProviderID returns the provider's numeric ID.
func (c *Context) ProviderID() types.ProviderID {
	return c.provider.ID
}

// ServiceURL returns the base URL of the provider's curio HTTP API.
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

// signHashFunc returns a closure that signs a 32-byte hash using c.signer.
// The closure indirects through [signer.SignHash] so the EVMSigner contract
// remains free of the dangerous SignHash method while internal SDK code can
// still produce EIP-712 signatures.
func (c *Context) signHashFunc() func([]byte) ([]byte, error) {
	return func(hash []byte) ([]byte, error) {
		return signer.SignHash(c.signer, hash)
	}
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

func randomUint256() (*big.Int, error) {
	var buf [32]byte
	if _, err := io.ReadFull(randReader, buf[:]); err != nil {
		return nil, fmt.Errorf("read random uint256: %w", err)
	}
	return new(big.Int).SetBytes(buf[:]), nil
}

func randomClientDataSetID() (types.ClientDataSetID, error) {
	v, err := randomUint256()
	if err != nil {
		return nil, fmt.Errorf("read random clientDataSetID: %w", err)
	}
	return v, nil
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

func (c *Context) rememberPresignedExtraDataLocked(extraData []byte, kind commitExtraDataKind) {
	if len(extraData) == 0 || kind != commitExtraDataCreateAndAdd {
		return
	}
	if c.presignedKinds == nil {
		c.presignedKinds = make(map[[32]byte]commitExtraDataKind)
	}
	c.presignedKinds[presignedExtraDataKey(extraData)] = kind
}

func (c *Context) presignedExtraDataIsStale(extraData []byte) bool {
	if len(extraData) == 0 {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	kind, ok := c.presignedKinds[presignedExtraDataKey(extraData)]
	if !ok {
		return false
	}
	if c.dataSetID == nil {
		return kind != commitExtraDataCreateAndAdd
	}
	return kind != commitExtraDataAddOnly
}

func (c *Context) forgetPresignedExtraData(extraData []byte) {
	if len(extraData) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.presignedKinds, presignedExtraDataKey(extraData))
	if len(c.presignedKinds) == 0 {
		c.presignedKinds = nil
	}
}

func presignedExtraDataKey(extraData []byte) [32]byte {
	return sha256.Sum256(extraData)
}
