package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"math/big"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

var (
	createDataSetArgs = mustABIArguments("address", "uint256", "string[]", "string[]", "bytes")
	addPiecesArgs     = mustABIArguments("uint256", "string[][]", "string[][]", "bytes")
	createAndAddArgs  = mustABIArguments("bytes", "bytes")
	bytesArgs         = mustABIArguments("bytes")
)

func mustABIArguments(typeNames ...string) abi.Arguments {
	args := make(abi.Arguments, len(typeNames))
	for i, typeName := range typeNames {
		t, err := abi.NewType(typeName, "", nil)
		if err != nil {
			panic("storage: failed to parse " + typeName + " ABI type: " + err.Error())
		}
		args[i] = abi.Argument{Type: t}
	}
	return args
}

var randReader io.Reader = rand.Reader

const (
	maxMetadataKeyLength   = 32
	maxMetadataValueLength = 96
	maxDataSetMetadataKeys = 10
	maxPieceMetadataKeys   = 3
)

// PDPProviderClient is the provider HTTP API surface required by storage contexts.
// It is injectable for tests and alternate provider clients.
type PDPProviderClient interface {
	UploadPieceStreaming(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error)
	DownloadPiece(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	WaitForPieceParked(context.Context, cid.Cid, time.Duration) error
	WaitForPullComplete(context.Context, pdp.PullRequest, time.Duration, func(*pdp.PullResult)) (*pdp.PullResult, error)
	AddPieces(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error)
	WaitForPiecesAdded(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	CreateDataSet(context.Context, common.Address, []byte) (*pdp.CreateDataSetResult, error)
	WaitForDataSetCreated(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error)
	CreateDataSetAndAddPieces(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error)
	WaitForCreateDataSetAndAddPieces(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	SchedulePieceDeletion(ctx context.Context, dataSetID, pieceID types.BigInt, extraData []byte) (common.Hash, error)
}

// Provider holds the on-chain identity of a storage provider.
type Provider struct {
	ID              types.BigInt   // provider ID from SPRegistry
	ServiceURL      string         // base URL of the provider's PDP HTTP API
	ServiceProvider common.Address // provider's EVM address
	Payee           common.Address // address that receives payments
}

// ContextOption configures provider and data-set contexts during construction.
type ContextOption func(*contextCore)

// ProviderContext represents one provider without a bound data set. Commit and
// Pull operations on a ProviderContext create a new data set. It is safe for
// concurrent use; concurrent create operations are independent.
type ProviderContext struct {
	core *contextCore
}

// DataSetContext represents one immutable provider + data-set target. Commit
// and Pull operations always target Ref. It is safe for concurrent use.
type DataSetContext struct {
	core *contextCore
	ref  DataSetRef
}

// contextCore contains the immutable configuration shared by provider and
// data-set contexts. Mutable inputs are copied before a core is published.
type contextCore struct {
	provider     Provider
	client       PDPProviderClient
	signer       signer.EVMSigner
	payer        common.Address
	chainID      types.ChainID
	recordKeeper common.Address
	withCDN      bool
	cdnRetriever CDNRetriever
	logger       *slog.Logger

	dataSetMetadata map[string]string

	// Optional read/write collaborators used by lifecycle methods that read
	// PDP/FWSS state (GetScheduledRemovals, PieceStatus, DeletePiece by CID,
	// Upload or Commit to an existing data set, Terminate).
	// All are nil by default; methods that require one return a descriptive
	// error when it is unset. Upload and Commit validate existing data sets
	// when a validator is configured.
	pdpCaller        PDPVerifierReader
	pdpConfig        PDPConfigReader
	fwssTerminator   FWSSTerminator
	dataSetValidator DataSetValidator
	dataSetReader    FWSSDataSetReader
	paymentReader    PaymentStateReader
	epochReader      EpochReader
	paymentToken     common.Address
}

// NewProviderContext creates an immutable context for the given provider and
// PDP client. Commit and Pull create a new data set.
// provider.ID, provider.ServiceURL, and client are validated here. Signing
// prerequisites (such as a non-nil signer plus chain/payer/record-keeper
// options) are validated by the write paths that need them, e.g.
// PresignForCommit.
func NewProviderContext(provider Provider, client PDPProviderClient, evmSigner signer.EVMSigner, opts ...ContextOption) (*ProviderContext, error) {
	if provider.ID.IsZero() {
		return nil, fmt.Errorf("storage.NewProviderContext: %w: zero provider ID", ErrInvalidArgument)
	}
	if provider.ServiceURL == "" {
		return nil, fmt.Errorf("storage.NewProviderContext: %w: empty provider service URL", ErrInvalidArgument)
	}
	if client == nil {
		return nil, fmt.Errorf("storage.NewProviderContext: %w: nil PDP client", ErrInvalidArgument)
	}
	core := &contextCore{
		provider: Provider{
			ID:              copyBigInt(provider.ID),
			ServiceURL:      provider.ServiceURL,
			ServiceProvider: provider.ServiceProvider,
			Payee:           provider.Payee,
		},
		client: client,
		signer: evmSigner,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(core)
		}
	}
	core.dataSetMetadata = cloneStringMap(core.dataSetMetadata)
	return &ProviderContext{core: core}, nil
}

// NewDataSetContext creates an immutable context bound to ref.
func NewDataSetContext(provider Provider, client PDPProviderClient, evmSigner signer.EVMSigner, ref DataSetRef, opts ...ContextOption) (*DataSetContext, error) {
	providerCtx, err := NewProviderContext(provider, client, evmSigner, opts...)
	if err != nil {
		return nil, err
	}
	dataSetCtx, err := providerCtx.ForDataSet(ref)
	if err != nil {
		return nil, fmt.Errorf("storage.NewDataSetContext: %w", err)
	}
	return dataSetCtx, nil
}

// ForDataSet returns a new immutable context bound to ref. The receiver is not
// modified and may continue to create independent data sets.
func (c *ProviderContext) ForDataSet(ref DataSetRef) (*DataSetContext, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.ProviderContext.ForDataSet: %w: nil context", ErrInvalidArgument)
	}
	if ref.ProviderID.IsZero() {
		return nil, fmt.Errorf("storage.ProviderContext.ForDataSet: %w: zero providerID", ErrInvalidArgument)
	}
	if !ref.ProviderID.Equal(c.core.provider.ID) {
		return nil, fmt.Errorf(
			"storage.ProviderContext.ForDataSet: %w: data set providerID %s does not match context providerID %s",
			ErrInvalidArgument,
			ref.ProviderID.String(),
			c.core.provider.ID.String(),
		)
	}
	if ref.DataSetID.IsZero() {
		return nil, fmt.Errorf("storage.ProviderContext.ForDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	return &DataSetContext{core: c.core, ref: copyDataSetRef(ref)}, nil
}

// WithPayer sets the EVM address that pays for storage.
func WithPayer(payer common.Address) ContextOption {
	return func(c *contextCore) { c.payer = payer }
}

// WithChainID sets the EIP-155 chain ID used for EIP-712 domain separation.
func WithChainID(chainID types.ChainID) ContextOption {
	return func(c *contextCore) { c.chainID = chainID }
}

// WithRecordKeeper sets the FWSS contract address (record-keeper) used for
// EIP-712 signing and passed to the PDP provider for Pull and dataset creation.
func WithRecordKeeper(addr common.Address) ContextOption {
	return func(c *contextCore) { c.recordKeeper = addr }
}

// WithDataSetMetadata sets the key-value metadata stored with the data set on creation.
func WithDataSetMetadata(metadata map[string]string) ContextOption {
	return func(c *contextCore) { c.dataSetMetadata = cloneStringMap(metadata) }
}

// WithCDN enables CDN services for the data set and CDN-first downloads when
// a retriever is configured. When true, a "withCDN" metadata marker is added
// to the EIP-712 dataset-creation message; the contract activates CDN and
// applies its configured lockup upon seeing it.
func WithCDN(enabled bool) ContextOption {
	return func(c *contextCore) { c.withCDN = enabled }
}

// WithCDNRetriever injects the optional CDN retriever used by context downloads.
func WithCDNRetriever(r CDNRetriever) ContextOption {
	return func(c *contextCore) { c.cdnRetriever = normalizeOptional(r) }
}

// WithLogger sets the logger used for internal warnings.
func WithLogger(logger *slog.Logger) ContextOption {
	return func(c *contextCore) { c.logger = logger }
}

// WithPDPVerifierReader injects a reader for PDPVerifier contract state.
// Required by [DataSetContext.GetScheduledRemovals],
// [DataSetContext.PieceStatus], and [DataSetContext.DeletePiece].
func WithPDPVerifierReader(r PDPVerifierReader) ContextOption {
	return func(c *contextCore) { c.pdpCaller = normalizeOptional(r) }
}

// WithPDPConfigReader injects a reader for FWSSView PDPConfig. Required by
// [DataSetContext.PieceStatus] for challenge-window math.
func WithPDPConfigReader(r PDPConfigReader) ContextOption {
	return func(c *contextCore) { c.pdpConfig = normalizeOptional(r) }
}

// WithFWSSTerminator injects the terminator used by [DataSetContext.Terminate].
func WithFWSSTerminator(t FWSSTerminator) ContextOption {
	return func(c *contextCore) { c.fwssTerminator = normalizeOptional(t) }
}

// WithFWSSDataSetReader injects the reader used before uploads to reject
// existing data sets whose PDP payment rail has ended.
func WithFWSSDataSetReader(r FWSSDataSetReader) ContextOption {
	return func(c *contextCore) { c.dataSetReader = normalizeOptional(r) }
}

// WithDataSetValidator injects the validator used before uploading or
// committing pieces to an existing data set.
func WithDataSetValidator(v DataSetValidator) ContextOption {
	return func(c *contextCore) { c.dataSetValidator = normalizeOptional(v) }
}

// WithPaymentStateReader injects payment readers for provider-relayed
// termination debt pre-checks.
func WithPaymentStateReader(pay PaymentStateReader, epochs EpochReader, token common.Address) ContextOption {
	return func(c *contextCore) {
		c.paymentReader = normalizeOptional(pay)
		c.epochReader = normalizeOptional(epochs)
		c.paymentToken = token
	}
}

// Store streams data to the provider and waits for it to be parked.
func (c *ProviderContext) Store(ctx context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
	return c.core.store(ctx, "storage.ProviderContext.Store", r, opts)
}

// Store streams data to the provider and waits for it to be parked.
func (c *DataSetContext) Store(ctx context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
	return c.core.store(ctx, "storage.DataSetContext.Store", r, opts)
}

// store streams data to the provider and waits for it to be parked.
// The reader is consumed in a single pass. If opts.PieceCID is defined,
// the client skips inline commP calculation; otherwise commP is computed
// during the upload via TeeReader. opts may be nil.
func (c *contextCore) store(ctx context.Context, op string, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
	if r == nil {
		return nil, fmt.Errorf("%s: %w: nil reader", op, ErrInvalidArgument)
	}
	if opts == nil {
		opts = &StoreOptions{}
	}
	if opts.PieceCID.Defined() {
		if _, err := piece.ParseV2(opts.PieceCID); err != nil {
			return nil, fmt.Errorf("%s: invalid PieceCID: %w", op, err)
		}
	}
	size := detectSize(r, opts.PieceCID)
	res, err := c.client.UploadPieceStreaming(ctx, r, pdp.UploadPieceStreamingOptions{
		Size:       size,
		PieceCID:   opts.PieceCID,
		OnProgress: opts.OnProgress,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: upload: %w", op, err)
	}
	if !res.PieceCID.Defined() {
		return nil, errors.New(op + ": upload returned undefined PieceCIDv2")
	}
	if _, err := piece.ParseV2(res.PieceCID); err != nil {
		return nil, fmt.Errorf("%s: upload returned invalid PieceCIDv2: %w", op, err)
	}
	if err := c.client.WaitForPieceParked(ctx, res.PieceCID, 0); err != nil {
		return nil, fmt.Errorf("%s: wait for parked: %w", op, err)
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

// PresignForCommit signs a create-and-add payload for a new data set.
func (c *ProviderContext) PresignForCommit(ctx context.Context, pieces []PieceInput) ([]byte, error) {
	return c.core.presignForCommit(ctx, "storage.ProviderContext.PresignForCommit", nil, pieces)
}

// PresignForCommit signs an add-pieces payload for the bound data set.
func (c *DataSetContext) PresignForCommit(ctx context.Context, pieces []PieceInput) ([]byte, error) {
	return c.core.presignForCommit(ctx, "storage.DataSetContext.PresignForCommit", &c.ref, pieces)
}

func (c *contextCore) presignForCommit(ctx context.Context, op string, ref *DataSetRef, pieces []PieceInput) ([]byte, error) {
	if len(pieces) == 0 {
		return nil, fmt.Errorf("%s: %w: no pieces provided", op, ErrInvalidArgument)
	}
	if err := validateAddPiecesBatch(op, len(pieces)); err != nil {
		return nil, err
	}
	if c.signer == nil {
		return nil, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	if !c.chainID.IsValid() {
		return nil, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if c.recordKeeper == (common.Address{}) {
		return nil, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	if c.payer == (common.Address{}) {
		return nil, fmt.Errorf("%s: %w: zero payer", op, ErrInvalidArgument)
	}

	pieceCIDs := make([]cid.Cid, 0, len(pieces))
	pieceMetadata := make([][]ityped.MetadataEntry, 0, len(pieces))
	for _, p := range pieces {
		if !p.PieceCID.Defined() {
			return nil, fmt.Errorf("%s: %w: undefined pieceCID", op, ErrInvalidArgument)
		}
		pieceCIDs = append(pieceCIDs, p.PieceCID)
		meta, err := pieceMetadataEntries(p.PieceMetadata)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		pieceMetadata = append(pieceMetadata, meta)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	domain := ityped.NewDomain(c.chainID.BigInt(), c.recordKeeper)

	if ref != nil {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		nonce, err := randomUint256()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		sig, err := ityped.SignAddPieces(c.signHashFunc(), domain, ref.ClientDataSetID.Big(), nonce, pieceCIDs, pieceMetadata)
		if err != nil {
			if errors.Is(err, signer.ErrUnsupportedSigner) {
				return nil, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
			}
			return nil, fmt.Errorf("%s: sign add pieces: %w", op, err)
		}
		return encodeAddPiecesExtraData(nonce, pieceMetadata, signatureBytes(sig))
	}

	clientDataSetID, err := randomClientDataSetID()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	dataSetMetadata, err := dataSetMetadataEntries(c.dataSetMetadata, c.withCDN)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	createSig, err := ityped.SignCreateDataSet(c.signHashFunc(), domain, clientDataSetID.Big(), c.provider.Payee, dataSetMetadata)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
		}
		return nil, fmt.Errorf("%s: sign create dataset: %w", op, err)
	}
	nonce, err := randomUint256()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	addSig, err := ityped.SignAddPieces(c.signHashFunc(), domain, clientDataSetID.Big(), nonce, pieceCIDs, pieceMetadata)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
		}
		return nil, fmt.Errorf("%s: sign add pieces: %w", op, err)
	}
	createPayload, err := encodeCreateDataSetExtraData(c.payer, clientDataSetID.Big(), dataSetMetadata, signatureBytes(createSig))
	if err != nil {
		return nil, err
	}
	addPayload, err := encodeAddPiecesExtraData(nonce, pieceMetadata, signatureBytes(addSig))
	if err != nil {
		return nil, err
	}
	return encodeCreateAndAddExtraData(createPayload, addPayload)
}

// Pull asks this provider to fetch pieces for a new data set.
func (c *ProviderContext) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	return c.core.pull(ctx, "storage.ProviderContext.Pull", nil, req)
}

// Pull asks this provider to fetch pieces for the bound data set.
func (c *DataSetContext) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	return c.core.pull(ctx, "storage.DataSetContext.Pull", &c.ref, req)
}

func (c *contextCore) pull(ctx context.Context, op string, ref *DataSetRef, req PullRequest) (*PullResult, error) {
	if len(req.Pieces) == 0 {
		return nil, fmt.Errorf("%s: %w: no pieces provided", op, ErrInvalidArgument)
	}
	if err := validateAddPiecesBatch(op, len(req.Pieces)); err != nil {
		return nil, err
	}
	if req.From == nil {
		return nil, fmt.Errorf("%s: %w: nil source resolver", op, ErrInvalidArgument)
	}
	pdpReq := pdp.PullRequest{
		ExtraData:    append([]byte(nil), req.ExtraData...),
		RecordKeeper: c.recordKeeper,
	}
	if ref != nil {
		id := copyBigInt(ref.DataSetID)
		pdpReq.DataSetID = &id
	}

	pieceByString := make(map[string]cid.Cid, len(req.Pieces))
	for _, pieceCID := range req.Pieces {
		if !pieceCID.Defined() {
			return nil, fmt.Errorf("%s: %w: undefined pieceCID", op, ErrInvalidArgument)
		}
		sourceURL := req.From(pieceCID)
		if sourceURL == "" {
			return nil, fmt.Errorf("%s: %w: empty source URL", op, ErrInvalidArgument)
		}
		pdpReq.Pieces = append(pdpReq.Pieces, pdp.PullPieceInput{
			PieceCID:  pieceCID,
			SourceURL: sourceURL,
		})
		pieceByString[pieceCID.String()] = pieceCID
	}

	res, err := c.client.WaitForPullComplete(ctx, pdpReq, 0, func(snapshot *pdp.PullResult) {
		if req.OnProgress == nil {
			return
		}
		for _, pieceStatus := range snapshot.Pieces {
			pieceCID, ok := pieceByString[pieceStatus.PieceCID]
			if !ok {
				continue
			}
			req.OnProgress(pieceCID, PullStatus(pieceStatus.Status))
		}
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	out := &PullResult{Status: PullStatus(res.Status)}
	for _, pieceStatus := range res.Pieces {
		pieceCID, ok := pieceByString[pieceStatus.PieceCID]
		if !ok {
			continue
		}
		out.Pieces = append(out.Pieces, PullPieceResult{
			PieceCID: pieceCID,
			Status:   PullStatus(pieceStatus.Status),
		})
	}
	return out, nil
}

// Commit creates a data set, adds pieces, and waits for confirmation.
func (c *ProviderContext) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	return c.core.commit(ctx, "storage.ProviderContext.Commit", nil, req)
}

// Commit adds pieces to the bound data set and waits for confirmation.
func (c *DataSetContext) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	return c.core.commit(ctx, "storage.DataSetContext.Commit", &c.ref, req)
}

func (c *contextCore) commit(ctx context.Context, op string, ref *DataSetRef, req CommitRequest) (*CommitResult, error) {
	if len(req.Pieces) == 0 {
		return nil, fmt.Errorf("%s: %w: no pieces provided", op, ErrInvalidArgument)
	}
	if err := validateAddPiecesBatch(op, len(req.Pieces)); err != nil {
		return nil, err
	}
	if err := c.validateWritableDataSet(ctx, op, ref); err != nil {
		return nil, err
	}

	extraData := append([]byte(nil), req.ExtraData...)
	var err error
	if len(extraData) == 0 {
		extraData, err = c.presignForCommit(ctx, op, ref, req.Pieces)
		if err != nil {
			return nil, err
		}
	}

	pieces := make([]pdp.AddPieceInput, 0, len(req.Pieces))
	for _, p := range req.Pieces {
		pieces = append(pieces, pdp.AddPieceInput{PieceCID: p.PieceCID})
	}

	if ref != nil {
		added, err := c.client.AddPieces(ctx, ref.DataSetID, pieces, extraData)
		if err != nil {
			return nil, fmt.Errorf("%s: add pieces: %w", op, err)
		}
		if req.OnSubmitted != nil {
			req.OnSubmitted(added.TxHash.Hex())
		}
		status, err := c.client.WaitForPiecesAdded(ctx, added.StatusURL, 0)
		if err != nil {
			return nil, fmt.Errorf("%s: wait add pieces: %w", op, err)
		}
		if status.DataSetID.IsZero() {
			return nil, errors.New(op + ": server returned zero dataSetID")
		}
		if !status.DataSetID.Equal(ref.DataSetID) {
			return nil, fmt.Errorf("%s: server returned mismatched dataSetID: got %s want %s", op, status.DataSetID.String(), ref.DataSetID.String())
		}
		if err := validateConfirmedPieceIDs(status.ConfirmedPieceIDs, len(req.Pieces)); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		return &CommitResult{
			TransactionID: status.TxHash.Hex(),
			DataSetID:     status.DataSetID,
			PieceIDs:      append([]types.BigInt(nil), status.ConfirmedPieceIDs...),
			IsNewDataSet:  false,
		}, nil
	}

	created, err := c.client.CreateDataSetAndAddPieces(ctx, c.recordKeeper, pieces, extraData)
	if err != nil {
		return nil, fmt.Errorf("%s: create and add pieces: %w", op, err)
	}
	if req.OnSubmitted != nil {
		req.OnSubmitted(created.TxHash.Hex())
	}
	status, err := c.client.WaitForCreateDataSetAndAddPieces(ctx, created.StatusURL, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: wait create and add pieces: %w", op, err)
	}
	if status.DataSetID.IsZero() {
		return nil, errors.New(op + ": server returned zero dataSetID")
	}
	if err := validateConfirmedPieceIDs(status.ConfirmedPieceIDs, len(req.Pieces)); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &CommitResult{
		TransactionID: status.TxHash.Hex(),
		DataSetID:     status.DataSetID,
		PieceIDs:      append([]types.BigInt(nil), status.ConfirmedPieceIDs...),
		IsNewDataSet:  true,
	}, nil
}

func (c *contextCore) validateWritableDataSet(ctx context.Context, op string, ref *DataSetRef) error {
	if ref == nil {
		return nil
	}
	if c.dataSetReader != nil {
		info, err := c.dataSetReader.GetDataSet(ctx, ref.DataSetID)
		if err != nil {
			return fmt.Errorf("%s: validate data set %s: %w", op, ref.DataSetID.String(), err)
		}
		if info == nil {
			return fmt.Errorf("%s: %w: FWSS returned no data set for dataSetID %s", op, ErrInvalidArgument, ref.DataSetID.String())
		}
		if err := validateDataSetAcceptsUploads(ref.DataSetID, info.PDPEndEpoch); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
	if c.dataSetValidator != nil {
		if err := c.dataSetValidator.ValidateDataSet(ctx, ref.DataSetID); err != nil {
			return fmt.Errorf("%s: validate data set %s: %w", op, ref.DataSetID.String(), err)
		}
	}
	return nil
}

// PieceURL returns the HTTPS retrieval URL for the given piece CID on this provider.
func (c *ProviderContext) PieceURL(pieceCID cid.Cid) string {
	return c.core.pieceURLFor(pieceCID)
}

// PieceURL returns the HTTPS retrieval URL for the given piece CID on this provider.
func (c *DataSetContext) PieceURL(pieceCID cid.Cid) string {
	return c.core.pieceURLFor(pieceCID)
}

// ProviderID returns the provider's numeric ID.
func (c *ProviderContext) ProviderID() types.BigInt {
	return copyBigInt(c.core.provider.ID)
}

// ProviderID returns the provider's numeric ID.
func (c *DataSetContext) ProviderID() types.BigInt {
	return copyBigInt(c.core.provider.ID)
}

// GetProviderInfo returns a copy of the provider configuration.
func (c *ProviderContext) GetProviderInfo() Provider {
	return c.core.providerInfo()
}

// GetProviderInfo returns a copy of the provider configuration.
func (c *DataSetContext) GetProviderInfo() Provider {
	return c.core.providerInfo()
}

func (c *contextCore) providerInfo() Provider {
	return Provider{
		ID:              copyBigInt(c.provider.ID),
		ServiceURL:      c.provider.ServiceURL,
		ServiceProvider: c.provider.ServiceProvider,
		Payee:           c.provider.Payee,
	}
}

// ServiceURL returns the base URL of the provider's PDP HTTP API.
func (c *ProviderContext) ServiceURL() string {
	return c.core.provider.ServiceURL
}

// ServiceURL returns the base URL of the provider's PDP HTTP API.
func (c *DataSetContext) ServiceURL() string {
	return c.core.provider.ServiceURL
}

// DataSetRef reports that a ProviderContext is not bound to a data set.
func (c *ProviderContext) DataSetRef() (DataSetRef, bool) {
	return DataSetRef{}, false
}

// DataSetRef returns the immutable target of this context.
func (c *DataSetContext) DataSetRef() (DataSetRef, bool) {
	return copyDataSetRef(c.ref), true
}

// DataSetID returns the immutable on-chain data-set ID.
func (c *DataSetContext) DataSetID() types.BigInt {
	return copyBigInt(c.ref.DataSetID)
}

// ClientDataSetID returns the immutable client-chosen data-set ID.
func (c *DataSetContext) ClientDataSetID() types.BigInt {
	return copyBigInt(c.ref.ClientDataSetID)
}

// CDNEnabled reports whether CDN services are enabled for this context.
func (c *ProviderContext) CDNEnabled() bool {
	return c.core.withCDN
}

// CDNEnabled reports whether CDN services are enabled for this context.
func (c *DataSetContext) CDNEnabled() bool {
	return c.core.withCDN
}

// WithCDN reports whether CDN services are enabled for this context.
//
// Deprecated: use CDNEnabled.
func (c *ProviderContext) WithCDN() bool {
	return c.CDNEnabled()
}

// WithCDN reports whether CDN services are enabled for this context.
//
// Deprecated: use CDNEnabled.
func (c *DataSetContext) WithCDN() bool {
	return c.CDNEnabled()
}

func (c *contextCore) pieceURLFor(pieceCID cid.Cid) string {
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
func (c *contextCore) signHashFunc() func([]byte) ([]byte, error) {
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

func validateAddPiecesBatch(op string, count int) error {
	if count > pdp.MaxAddPiecesBatchSize {
		return fmt.Errorf("%s: %w: %w: got %d, max %d", op, ErrInvalidArgument, pdp.ErrTooManyPieces, count, pdp.MaxAddPiecesBatchSize)
	}
	return nil
}

func encodeCreateDataSetExtraData(payer common.Address, clientDataSetID *big.Int, metadata []ityped.MetadataEntry, signature []byte) ([]byte, error) {
	keys := make([]string, 0, len(metadata))
	values := make([]string, 0, len(metadata))
	for _, m := range metadata {
		keys = append(keys, m.Key)
		values = append(values, m.Value)
	}
	out, err := createDataSetArgs.Pack(payer, clientDataSetID, keys, values, signature)
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
	out, err := addPiecesArgs.Pack(nonce, keys, values, signature)
	if err != nil {
		return nil, fmt.Errorf("storage: encode add pieces extraData: %w", err)
	}
	return out, nil
}

func encodeCreateAndAddExtraData(createPayload, addPayload []byte) ([]byte, error) {
	out, err := createAndAddArgs.Pack(createPayload, addPayload)
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

func randomClientDataSetID() (types.BigInt, error) {
	v, err := randomUint256()
	if err != nil {
		return types.BigInt{}, fmt.Errorf("read random clientDataSetID: %w", err)
	}
	id, err := types.BigIntFromBig(v)
	if err != nil {
		return types.BigInt{}, fmt.Errorf("read random clientDataSetID: %w", err)
	}
	return id, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

func copyDataSetRef(ref DataSetRef) DataSetRef {
	return DataSetRef{
		ProviderID:      copyBigInt(ref.ProviderID),
		DataSetID:       copyBigInt(ref.DataSetID),
		ClientDataSetID: copyBigInt(ref.ClientDataSetID),
	}
}
