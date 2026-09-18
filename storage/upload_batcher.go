package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

const (
	defaultUploadIdleWait                 = 3 * time.Second
	defaultUploadMaxConcurrentSubmissions = 4
	defaultSharedDataSetPollInterval      = 4 * time.Second
	defaultSharedDataSetVisibleTimeout    = 5 * time.Minute
)

// UploadBatcherOptions configures the fixed authorization identity of an
// UploadBatcher. The signer may be an authorized delegated key and need not
// have the same address as Identity.Payer.
type UploadBatcherOptions struct {
	Identity ContextIdentity
	Signer   signer.StorageSigner
}

type uploadBatcherConfig struct {
	idleWait                 time.Duration
	idleWaitEnabled          bool
	maxWait                  time.Duration
	maxWaitEnabled           bool
	maxConcurrentSubmissions int
	clock                    uploadBatchClock
	dataSetPollInterval      time.Duration
	dataSetVisibleTimeout    time.Duration
}

// UploadBatcherOption configures upload batching behavior.
type UploadBatcherOption func(*uploadBatcherConfig)

// WithUploadIdleWait sets how long a batch waits without a new piece before it
// is submitted. The wait starts once no other upload to the same target is in
// progress; zero submits as soon as that is true.
func WithUploadIdleWait(wait time.Duration) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) {
		cfg.idleWait = wait
		cfg.idleWaitEnabled = true
	}
}

// WithoutUploadIdleWait disables inactivity-based submission.
func WithoutUploadIdleWait() UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) { cfg.idleWaitEnabled = false }
}

// WithUploadMaxWait submits a batch at most wait after its first piece is
// ready, even while other uploads to the same target are in progress. Zero
// submits every piece immediately and requires a zero or disabled idle wait.
// There is no maximum wait by default.
func WithUploadMaxWait(wait time.Duration) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) {
		cfg.maxWait = wait
		cfg.maxWaitEnabled = true
	}
}

// WithoutUploadMaxWait removes the maximum wait, which is the default.
func WithoutUploadMaxWait() UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) { cfg.maxWaitEnabled = false }
}

// WithUploadMaxConcurrentSubmissions limits concurrent signing and provider
// submissions. Confirmation waits do not consume this limit.
func WithUploadMaxConcurrentSubmissions(max int) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) { cfg.maxConcurrentSubmissions = max }
}

type uploadBatchClock interface {
	Now() time.Time
	AfterFunc(time.Duration, func()) uploadBatchTimer
}

type uploadBatchTimer interface {
	Stop() bool
}

type systemUploadBatchClock struct{}

func (systemUploadBatchClock) Now() time.Time { return time.Now() }

func (systemUploadBatchClock) AfterFunc(wait time.Duration, fn func()) uploadBatchTimer {
	return time.AfterFunc(wait, fn)
}

func withUploadBatchClock(clock uploadBatchClock) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) { cfg.clock = clock }
}

func withSharedDataSetPolling(interval, timeout time.Duration) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) {
		cfg.dataSetPollInterval = interval
		cfg.dataSetVisibleTimeout = timeout
	}
}

// UploadBatcher coordinates compatible high-level uploads into bounded PDP
// add-pieces submissions. It is safe for concurrent use.
type UploadBatcher struct {
	identity ContextIdentity
	signer   signer.StorageSigner
	config   uploadBatcherConfig
	ctx      context.Context
	cancel   context.CancelCauseFunc

	mu              sync.Mutex
	closed          bool
	nextReservation uint64
	nextBatch       uint64
	nextFlush       uint64
	nextBinding     uint64
	reservations    map[uint64]chan struct{}
	boundContexts   map[uint64]context.CancelCauseFunc
	windows         map[string]*uploadBatchWindow
	flights         map[uint64]*uploadBatchFlight
	failedFlights   map[uint64]*uploadBatchFailure
	flushes         map[uint64]*uploadBatchFlush
	datasetGates    map[string]*uploadBatchDataSetGate
	newTargets      map[string]*uploadBatchNewTarget
	transfers       map[string]int
	submissionSlots chan struct{}
}

type uploadBatchNewTargetPhase int

const (
	uploadBatchNewTargetIdle uploadBatchNewTargetPhase = iota
	uploadBatchNewTargetLookingUp
	uploadBatchNewTargetCreating
	uploadBatchNewTargetSubmitted
)

// uploadBatchNewTarget is the data set shared by one new-target key. Its client
// data-set ID stays fixed across create retries, so at most one create succeeds.
type uploadBatchNewTarget struct {
	clientDataSetID types.BigInt
	ref             *DataSetRef
	refVisible      bool
	phase           uploadBatchNewTargetPhase
	needsLookup     bool
	changed         chan struct{}
}

func (t *uploadBatchNewTarget) notifyLocked() {
	close(t.changed)
	t.changed = make(chan struct{})
}

// uploadBatchUnboundTarget can join the data set shared by its new-target key.
type uploadBatchUnboundTarget interface {
	StorageContext
	forDataSet(DataSetRef) (StorageContext, error)
	findDataSetByClientDataSetID(context.Context, types.BigInt) (DataSetRef, bool, error)
}

func (c *ProviderContext) forDataSet(ref DataSetRef) (StorageContext, error) {
	dataSetCtx, err := c.ForDataSet(ref)
	if err != nil {
		return nil, err
	}
	return dataSetCtx, nil
}

func (c *ProviderContext) findDataSetByClientDataSetID(ctx context.Context, clientDataSetID types.BigInt) (DataSetRef, bool, error) {
	return c.FindDataSetByClientDataSetID(ctx, clientDataSetID)
}

type uploadBatchSharedPlan struct {
	state           *uploadBatchNewTarget
	target          StorageContext
	ref             *DataSetRef
	clientDataSetID types.BigInt
}

type uploadBatchDataSetGate struct {
	slot chan struct{}
	refs int
}

type uploadBatchWindow struct {
	key             string
	target          StorageContext
	ref             *DataSetRef
	slots           []*uploadBatchSlot
	pieceCIDs       map[string]struct{}
	idleDeadline    time.Time
	maxDeadline     time.Time
	timer           uploadBatchTimer
	timerGeneration uint64
}

type uploadBatchSlot struct {
	reservation uint64
	piece       PieceInput
	readyAt     time.Time
	events      chan uploadBatchEvent
	done        bool
	observed    bool
	flight      *uploadBatchFlight
}

type uploadBatchEvent struct {
	submission *CommitSubmission
	result     *CommitResult
	err        error
	final      bool
}

type uploadBatchTask struct {
	batcher *UploadBatcher
	slot    *uploadBatchSlot
}

type uploadBatchFlight struct {
	seq            uint64
	minReservation uint64
	key            string
	target         StorageContext
	ref            *DataSetRef
	slots          []*uploadBatchSlot
	done           chan struct{}
	err            error
}

// uploadBatchFailure is the Flush-visible remainder of a finished error.
// It deliberately omits the flight's target, pieces, and slots so an
// unobserved failure does not pin that memory until the next Flush or Close.
type uploadBatchFailure struct {
	seq            uint64
	minReservation uint64
	err            error
}

type uploadBatchFlush struct {
	barrier  uint64
	batches  map[uint64]*uploadBatchFlight
	failures map[uint64]*uploadBatchFailure
}

type uploadReservation struct {
	batcher *UploadBatcher
	seq     uint64
	once    sync.Once

	// released and transfers are guarded by batcher.mu.
	released  bool
	transfers []*uploadBatchTransfer
}

// uploadBatchTransfer marks a Store or Pull that may still join a target's
// window. While any transfer for a target is open, that window's idle wait
// does not start.
type uploadBatchTransfer struct {
	batcher *UploadBatcher
	key     string
	ended   bool // guarded by batcher.mu
}

// NewUploadBatcher creates an independent batching coordinator. By default, a
// batch is submitted three seconds after its last new piece once no other
// upload to the same target is in progress, with no maximum wait, and up to
// four batches may be signed and submitted concurrently. When options of the
// same kind are repeated, the last one takes effect.
func NewUploadBatcher(opts UploadBatcherOptions, options ...UploadBatcherOption) (*UploadBatcher, error) {
	const op = "storage.NewUploadBatcher"
	storageSigner := normalizeOptional(opts.Signer)
	if !identityComplete(opts.Identity) {
		return nil, fmt.Errorf("%s: %w: incomplete identity", op, ErrInvalidArgument)
	}
	if storageSigner == nil {
		return nil, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	if storageSigner.EVMAddress() == (common.Address{}) {
		return nil, fmt.Errorf("%s: %w: zero signer address", op, ErrInvalidArgument)
	}
	cfg := uploadBatcherConfig{
		idleWait:                 defaultUploadIdleWait,
		idleWaitEnabled:          true,
		maxConcurrentSubmissions: defaultUploadMaxConcurrentSubmissions,
		clock:                    systemUploadBatchClock{},
		dataSetPollInterval:      defaultSharedDataSetPollInterval,
		dataSetVisibleTimeout:    defaultSharedDataSetVisibleTimeout,
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if cfg.clock == nil {
		return nil, fmt.Errorf("%s: %w: nil clock", op, ErrInvalidArgument)
	}
	if cfg.idleWaitEnabled && cfg.idleWait < 0 {
		return nil, fmt.Errorf("%s: %w: negative idle wait", op, ErrInvalidArgument)
	}
	if cfg.maxWaitEnabled && cfg.maxWait < 0 {
		return nil, fmt.Errorf("%s: %w: negative max wait", op, ErrInvalidArgument)
	}
	if cfg.idleWaitEnabled && cfg.maxWaitEnabled && cfg.idleWait > cfg.maxWait {
		return nil, fmt.Errorf("%s: %w: idle wait exceeds max wait", op, ErrInvalidArgument)
	}
	if cfg.maxConcurrentSubmissions < 1 {
		return nil, fmt.Errorf("%s: %w: max concurrent submissions must be positive", op, ErrInvalidArgument)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &UploadBatcher{
		identity:        opts.Identity,
		signer:          storageSigner,
		config:          cfg,
		ctx:             ctx,
		cancel:          cancel,
		reservations:    make(map[uint64]chan struct{}),
		boundContexts:   make(map[uint64]context.CancelCauseFunc),
		windows:         make(map[string]*uploadBatchWindow),
		flights:         make(map[uint64]*uploadBatchFlight),
		failedFlights:   make(map[uint64]*uploadBatchFailure),
		flushes:         make(map[uint64]*uploadBatchFlush),
		datasetGates:    make(map[string]*uploadBatchDataSetGate),
		newTargets:      make(map[string]*uploadBatchNewTarget),
		transfers:       make(map[string]int),
		submissionSlots: make(chan struct{}, cfg.maxConcurrentSubmissions),
	}, nil
}

func (b *UploadBatcher) reserve() (*uploadReservation, error) {
	if b == nil {
		return nil, fmt.Errorf("storage.UploadBatcher.reserve: %w: nil batcher", ErrInvalidArgument)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	b.nextReservation++
	done := make(chan struct{})
	b.reservations[b.nextReservation] = done
	return &uploadReservation{batcher: b, seq: b.nextReservation}, nil
}

func (r *uploadReservation) release() {
	if r == nil || r.batcher == nil {
		return
	}
	r.once.Do(func() {
		b := r.batcher
		b.mu.Lock()
		if done, ok := b.reservations[r.seq]; ok {
			delete(b.reservations, r.seq)
			close(done)
		}
		r.released = true
		launches := make([]*uploadBatchFlight, 0, len(r.transfers))
		for _, transfer := range r.transfers {
			launches = append(launches, b.endTransferLocked(transfer, true))
		}
		r.transfers = nil
		b.mu.Unlock()
		b.launchFlights(launches)
	})
}

// beginTransfer records a Store or Pull whose piece may join target's window.
// The transfer ends when its piece is enqueued, when end is called, or when the
// reservation is released, so an upload never keeps a target open while it
// waits for commit results.
func (r *uploadReservation) beginTransfer(target StorageContext) (*uploadBatchTransfer, error) {
	const op = "storage.UploadBatcher.beginTransfer"
	if r == nil || r.batcher == nil {
		return nil, fmt.Errorf("%s: %w: nil reservation", op, ErrInvalidArgument)
	}
	key, _, err := uploadBatchTargetKey(target)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	b := r.batcher
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	if r.released {
		return nil, fmt.Errorf("%s: %w: released reservation", op, ErrInvalidArgument)
	}
	return b.beginTransferLocked(r, key), nil
}

func (b *UploadBatcher) beginTransferLocked(r *uploadReservation, key string) *uploadBatchTransfer {
	transfer := &uploadBatchTransfer{batcher: b, key: key}
	b.transfers[key]++
	r.transfers = append(r.transfers, transfer)
	if window := b.windows[key]; window != nil {
		b.scheduleWindowLocked(window, b.config.clock.Now())
	}
	return transfer
}

// end marks a transfer that will not enqueue a piece. It is idempotent.
func (t *uploadBatchTransfer) end() {
	if t == nil || t.batcher == nil {
		return
	}
	b := t.batcher
	b.mu.Lock()
	flight := b.endTransferLocked(t, true)
	b.mu.Unlock()
	b.launchFlights([]*uploadBatchFlight{flight})
}

// endTransferLocked ends transfer. With reschedule, the last transfer for a
// target restarts that window's idle wait, or submits it when the idle wait is
// zero.
func (b *UploadBatcher) endTransferLocked(transfer *uploadBatchTransfer, reschedule bool) *uploadBatchFlight {
	if transfer == nil || transfer.ended {
		return nil
	}
	transfer.ended = true
	if b.closed {
		return nil
	}
	if count := b.transfers[transfer.key]; count > 1 {
		b.transfers[transfer.key] = count - 1
		return nil
	}
	delete(b.transfers, transfer.key)
	window := b.windows[transfer.key]
	if !reschedule || window == nil || !b.config.idleWaitEnabled {
		return nil
	}
	now := b.config.clock.Now()
	window.idleDeadline = now.Add(b.config.idleWait)
	if b.config.idleWait == 0 {
		return b.sealWindowLocked(window)
	}
	b.scheduleWindowLocked(window, now)
	return nil
}

func (b *UploadBatcher) bindContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		cancel(ErrClosed)
		return ctx, func() { cancel(context.Canceled) }
	}
	b.nextBinding++
	id := b.nextBinding
	b.boundContexts[id] = cancel
	b.mu.Unlock()
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.boundContexts, id)
			b.mu.Unlock()
			cancel(context.Canceled)
		})
	}
}

// uploadBatchContextError maps a cancelled bound context to ErrClosed or
// ctx.Err() when fallback is absent or is itself a context error. A published
// terminal error is left unchanged so callers can match it after cancellation.
func uploadBatchContextError(ctx context.Context, fallback error) error {
	if errors.Is(fallback, ErrClosed) {
		return ErrClosed
	}
	if fallback != nil && !isContextError(fallback) {
		return fallback
	}
	if ctx != nil {
		if errors.Is(context.Cause(ctx), ErrClosed) {
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return fallback
}

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (b *UploadBatcher) validateTarget(target StorageContext) error {
	if isNilStorageContext(target) {
		return fmt.Errorf("storage.UploadBatcher: %w: nil storage context", ErrInvalidArgument)
	}
	if target.ContextIdentity() != b.identity {
		return fmt.Errorf("storage.UploadBatcher: %w: context identity does not match batcher identity", ErrInvalidArgument)
	}
	if _, bound := target.DataSetRef(); !bound {
		if _, ok := target.(uploadBatchUnboundTarget); !ok {
			return fmt.Errorf("storage.UploadBatcher: %w: unbound context cannot join a shared data set", ErrInvalidArgument)
		}
	}
	return nil
}

func (b *UploadBatcher) newTargetLocked(key string) (*uploadBatchNewTarget, error) {
	if state := b.newTargets[key]; state != nil {
		return state, nil
	}
	clientDataSetID, err := randomClientDataSetID()
	if err != nil {
		return nil, err
	}
	state := &uploadBatchNewTarget{clientDataSetID: clientDataSetID, changed: make(chan struct{})}
	b.newTargets[key] = state
	return state, nil
}

// enqueue adds piece to target's window. A non-nil transfer always ends,
// whether or not the piece is accepted.
func (b *UploadBatcher) enqueue(ctx context.Context, reservation uint64, target StorageContext, piece PieceInput, transfer *uploadBatchTransfer) (*uploadBatchTask, error) {
	const op = "storage.UploadBatcher.enqueue"
	defer transfer.end()
	if ctx == nil {
		return nil, fmt.Errorf("%s: %w: nil context", op, ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := b.validateTarget(target); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if _, err := validateCommitPieces(op, []PieceInput{piece}); err != nil {
		return nil, err
	}
	key, ref, err := uploadBatchTargetKey(target)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var launches []*uploadBatchFlight
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	var sizeClientDataSetID *types.BigInt
	if ref == nil {
		state, stateErr := b.newTargetLocked(key)
		if stateErr != nil {
			b.mu.Unlock()
			return nil, fmt.Errorf("%s: %w", op, stateErr)
		}
		id := copyBigInt(state.clientDataSetID)
		sizeClientDataSetID = &id
	}
	window := b.windows[key]
	if window != nil {
		pieces := append(windowPieces(window), clonePieceInput(piece))
		_, duplicate := window.pieceCIDs[canonicalCommitPieceCIDKey(piece.PieceCID)]
		if duplicate || b.validateCandidate(window.target, window.ref, sizeClientDataSetID, pieces) != nil {
			launches = append(launches, b.sealWindowLocked(window))
			window = nil
		}
	}
	if window == nil {
		window = &uploadBatchWindow{
			key:       key,
			target:    target,
			ref:       copyDataSetRefPtr(ref),
			pieceCIDs: make(map[string]struct{}),
		}
		if err := b.validateCandidate(target, window.ref, sizeClientDataSetID, []PieceInput{piece}); err != nil {
			b.mu.Unlock()
			b.launchFlights(launches)
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		b.windows[key] = window
	}
	now := b.config.clock.Now()
	slot := &uploadBatchSlot{
		reservation: reservation,
		piece:       clonePieceInput(piece),
		readyAt:     now,
		events:      make(chan uploadBatchEvent, 2),
	}
	window.slots = append(window.slots, slot)
	window.pieceCIDs[canonicalCommitPieceCIDKey(piece.PieceCID)] = struct{}{}
	if len(window.slots) == 1 && b.config.maxWaitEnabled {
		window.maxDeadline = now.Add(b.config.maxWait)
	}
	if b.config.idleWaitEnabled {
		window.idleDeadline = now.Add(b.config.idleWait)
	}
	if transfer != nil {
		// The piece is already in the window, so ending its own transfer here
		// needs no separate reschedule when the transfer used this target.
		launches = append(launches, b.endTransferLocked(transfer, transfer.key != key))
	}
	immediate := b.config.idleWaitEnabled && b.config.idleWait == 0 && b.transfers[key] == 0 ||
		b.config.maxWaitEnabled && b.config.maxWait == 0
	if immediate || len(window.slots) == pdp.MaxAddPiecesBatchSize {
		launches = append(launches, b.sealWindowLocked(window))
	} else {
		b.scheduleWindowLocked(window, now)
	}
	b.mu.Unlock()

	task := &uploadBatchTask{batcher: b, slot: slot}
	b.launchFlights(launches)
	return task, nil
}

func (b *UploadBatcher) validateCandidate(target StorageContext, ref *DataSetRef, clientDataSetID *types.BigInt, pieces []PieceInput) error {
	pieceCIDs, err := validateCommitPieces("storage.UploadBatcher", pieces)
	if err != nil {
		return err
	}
	pieceMetadata := make([][]ityped.MetadataEntry, 0, len(pieces))
	for _, piece := range pieces {
		metadata, metadataErr := pieceMetadataEntries(piece.PieceMetadata)
		if metadataErr != nil {
			return metadataErr
		}
		pieceMetadata = append(pieceMetadata, metadata)
	}
	pieceMetadata = ityped.CompactPieceMetadata(pieceMetadata)
	var extraData []byte
	if ref != nil {
		extraData, err = encodeAddPiecesExtraData(new(big.Int), pieceMetadata, make([]byte, secp256k1SignatureSize))
	} else {
		if clientDataSetID == nil {
			return fmt.Errorf("%w: missing client data-set ID", ErrInvalidArgument)
		}
		metadata, metadataErr := dataSetMetadataEntries(target.DataSetMetadata(), target.CDNEnabled())
		if metadataErr != nil {
			return metadataErr
		}
		createPayload, createErr := encodeCreateDataSetExtraData(b.identity.Payer, clientDataSetID.Big(), metadata, make([]byte, secp256k1SignatureSize))
		if createErr != nil {
			return createErr
		}
		addPayload, addErr := encodeAddPiecesExtraData(new(big.Int), pieceMetadata, make([]byte, secp256k1SignatureSize))
		if addErr != nil {
			return addErr
		}
		extraData, err = encodeCreateAndAddExtraData(createPayload, addPayload)
	}
	if err != nil {
		return err
	}
	return validateAddPiecesMessageSize("storage.UploadBatcher", pieceCIDs, extraData)
}

func uploadBatchTargetKey(target StorageContext) (string, *DataSetRef, error) {
	providerID := target.ProviderID()
	if providerID.IsZero() {
		return "", nil, fmt.Errorf("%w: zero provider ID", ErrInvalidArgument)
	}
	if target.ServiceURL() == "" {
		return "", nil, fmt.Errorf("%w: empty service URL", ErrInvalidArgument)
	}
	if ref, ok := target.DataSetRef(); ok {
		if !ref.valid() || !ref.ProviderID().Equal(providerID) {
			return "", nil, fmt.Errorf("%w: invalid data-set target", ErrInvalidArgument)
		}
		key := strings.Join([]string{
			"existing",
			providerID.String(),
			ref.DataSetID().String(),
			ref.ClientDataSetID().String(),
			lengthPrefixed(target.ServiceURL()),
		}, "\x00")
		return key, &ref, nil
	}
	provider := target.GetProviderInfo()
	if provider.ID.IsZero() || !provider.ID.Equal(providerID) {
		return "", nil, fmt.Errorf("%w: provider info does not match provider ID", ErrInvalidArgument)
	}
	if provider.Payee == (common.Address{}) {
		return "", nil, fmt.Errorf("%w: zero provider payee", ErrInvalidArgument)
	}
	metadata := target.DataSetMetadata()
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{"new", providerID.String(), lengthPrefixed(target.ServiceURL()), provider.Payee.Hex(), strconv.FormatBool(target.CDNEnabled())}
	for _, key := range keys {
		parts = append(parts, lengthPrefixed(key), lengthPrefixed(metadata[key]))
	}
	return strings.Join(parts, "\x00"), nil, nil
}

func lengthPrefixed(value string) string { return strconv.Itoa(len(value)) + ":" + value }

func clonePieceInput(piece PieceInput) PieceInput {
	return PieceInput{PieceCID: piece.PieceCID, PieceMetadata: cloneStringMap(piece.PieceMetadata)}
}

func windowPieces(window *uploadBatchWindow) []PieceInput {
	pieces := make([]PieceInput, len(window.slots))
	for i, slot := range window.slots {
		pieces[i] = clonePieceInput(slot.piece)
	}
	return pieces
}

func windowMinReservation(window *uploadBatchWindow) uint64 {
	minimum := window.slots[0].reservation
	for _, slot := range window.slots[1:] {
		if slot.reservation < minimum {
			minimum = slot.reservation
		}
	}
	return minimum
}

func (b *UploadBatcher) scheduleWindowLocked(window *uploadBatchWindow, now time.Time) {
	if window.timer != nil {
		window.timer.Stop()
		window.timer = nil
	}
	// A timer that already fired may be waiting for b.mu; a new generation
	// makes it a no-op even when no replacement timer is armed.
	window.timerGeneration++
	var deadline time.Time
	if b.config.idleWaitEnabled && b.transfers[window.key] == 0 {
		deadline = window.idleDeadline
	}
	if b.config.maxWaitEnabled && (deadline.IsZero() || window.maxDeadline.Before(deadline)) {
		deadline = window.maxDeadline
	}
	if deadline.IsZero() {
		return
	}
	generation := window.timerGeneration
	wait := max(deadline.Sub(now), 0)
	window.timer = b.config.clock.AfterFunc(wait, func() { b.fireWindowTimer(window, generation) })
}

func (b *UploadBatcher) fireWindowTimer(window *uploadBatchWindow, generation uint64) {
	b.mu.Lock()
	if b.closed || b.windows[window.key] != window || window.timerGeneration != generation {
		b.mu.Unlock()
		return
	}
	flight := b.sealWindowLocked(window)
	b.mu.Unlock()
	b.launchFlights([]*uploadBatchFlight{flight})
}

func (b *UploadBatcher) sealWindowLocked(window *uploadBatchWindow) *uploadBatchFlight {
	if window == nil || len(window.slots) == 0 || b.windows[window.key] != window {
		return nil
	}
	delete(b.windows, window.key)
	if window.timer != nil {
		window.timer.Stop()
		window.timer = nil
	}
	b.nextBatch++
	flight := &uploadBatchFlight{
		seq:            b.nextBatch,
		minReservation: window.slots[0].reservation,
		key:            window.key,
		target:         window.target,
		ref:            copyDataSetRefPtr(window.ref),
		slots:          append([]*uploadBatchSlot(nil), window.slots...),
		done:           make(chan struct{}),
	}
	for _, slot := range flight.slots {
		if slot.reservation < flight.minReservation {
			flight.minReservation = slot.reservation
		}
		slot.flight = flight
	}
	b.flights[flight.seq] = flight
	for _, flush := range b.flushes {
		if flight.minReservation <= flush.barrier {
			flush.batches[flight.seq] = flight
		}
	}
	return flight
}

func (b *UploadBatcher) launchFlights(flights []*uploadBatchFlight) {
	for _, flight := range flights {
		if flight != nil {
			go b.runFlight(flight)
		}
	}
}

func (b *UploadBatcher) runFlight(flight *uploadBatchFlight) {
	pieces := make([]PieceInput, len(flight.slots))
	for i, slot := range flight.slots {
		pieces[i] = clonePieceInput(slot.piece)
	}
	if flight.ref != nil {
		result, err := b.commitFlight(flight, pieces, flight.target, flight.ref, nil)
		b.finishFlight(flight, result, err)
		return
	}
	unbound, ok := flight.target.(uploadBatchUnboundTarget)
	if !ok {
		b.finishFlight(flight, nil, fmt.Errorf("storage.UploadBatcher: %w: unbound context cannot join a shared data set", ErrInvalidArgument))
		return
	}
	for {
		plan, err := b.resolveSharedDataSet(b.ctx, flight.key, unbound, true)
		if err != nil {
			b.finishFlight(flight, nil, uploadBatchContextError(b.ctx, err))
			return
		}
		if plan.ref != nil {
			result, err := b.commitFlight(flight, pieces, plan.target, plan.ref, nil)
			if err != nil && b.forgetTerminatedSharedDataSet(plan.state, *plan.ref, err) {
				continue
			}
			b.finishFlight(flight, result, err)
			return
		}
		result, err := b.commitFlight(flight, pieces, flight.target, nil, &plan)
		err = b.finishSharedCreate(plan.state, plan.clientDataSetID, flight.target.ProviderID(), result, err)
		if err != nil {
			result = nil
		}
		b.finishFlight(flight, result, err)
		return
	}
}

// commitFlight submits one batch; create is non-nil when it creates the shared
// data set.
func (b *UploadBatcher) commitFlight(flight *uploadBatchFlight, pieces []PieceInput, target StorageContext, ref *DataSetRef, create *uploadBatchSharedPlan) (*CommitResult, error) {
	var gate *uploadBatchDataSetGate
	var gateKey string
	if ref != nil {
		gateKey = uploadBatchGateKey(*ref, target.ServiceURL())
		b.mu.Lock()
		gate = b.datasetGates[gateKey]
		if gate == nil {
			gate = &uploadBatchDataSetGate{slot: make(chan struct{}, 1)}
			b.datasetGates[gateKey] = gate
		}
		gate.refs++
		b.mu.Unlock()
		if err := acquireUploadBatchSlot(b.ctx, gate.slot); err != nil {
			b.releaseDataSetGate(gateKey, gate, false)
			return nil, err
		}
	}
	if err := acquireUploadBatchSlot(b.ctx, b.submissionSlots); err != nil {
		if gate != nil {
			b.releaseDataSetGate(gateKey, gate, true)
		}
		return nil, err
	}
	var clientDataSetID *types.BigInt
	if create != nil {
		id := copyBigInt(create.clientDataSetID)
		clientDataSetID = &id
	}
	extraData, _, err := presignCommitAuthorization(b.ctx, "storage.UploadBatcher", commitAuthorization{
		identity:        b.identity,
		provider:        target.GetProviderInfo(),
		signer:          b.signer,
		dataSetMetadata: target.DataSetMetadata(),
		withCDN:         target.CDNEnabled(),
	}, ref, pieces, clientDataSetID)
	if err == nil {
		err = b.ctx.Err()
	}
	var submission *CommitSubmission
	if err == nil {
		submission, err = target.SubmitCommit(b.ctx, CommitRequest{
			Pieces:          pieces,
			ExtraData:       extraData,
			ClientDataSetID: copyBigIntPtr(clientDataSetID),
		})
	}
	if create != nil && err == nil {
		b.markSharedCreateSubmitted(create.state)
	}
	<-b.submissionSlots
	if gate != nil {
		b.releaseDataSetGate(gateKey, gate, true)
	}
	if err == nil {
		err = b.ctx.Err()
	}
	err = uploadBatchContextError(b.ctx, err)
	if err != nil {
		return nil, err
	}
	if submission == nil {
		return nil, errors.New("storage.UploadBatcher: submit commit returned nil submission")
	}
	if err := validateUploadBatchSubmissionPieces(pieces, submission.PieceCIDs); err != nil {
		return nil, err
	}
	for _, slot := range flight.slots {
		copySubmission := copyCommitSubmission(*submission)
		b.sendSlotEvent(slot, uploadBatchEvent{submission: &copySubmission})
	}
	result, err := target.WaitForCommit(b.ctx, *submission)
	err = uploadBatchContextError(b.ctx, err)
	if err == nil {
		if result == nil {
			err = errors.New("storage.UploadBatcher: wait for commit returned nil result")
		} else {
			err = validateConfirmedPieceIDs(result.PieceIDs, len(flight.slots))
		}
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveSharedDataSet waits until the shared data set is usable. A returned
// plan without ref makes a create=true caller the only creator, which must call
// finishSharedCreate.
func (b *UploadBatcher) resolveSharedDataSet(ctx context.Context, key string, unbound uploadBatchUnboundTarget, create bool) (uploadBatchSharedPlan, error) {
	for {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return uploadBatchSharedPlan{}, ErrClosed
		}
		state, err := b.newTargetLocked(key)
		if err != nil {
			b.mu.Unlock()
			return uploadBatchSharedPlan{}, err
		}
		idle := state.phase == uploadBatchNewTargetIdle
		switch {
		case state.ref != nil && state.refVisible:
			ref := copyDataSetRef(*state.ref)
			b.mu.Unlock()
			target, err := unbound.forDataSet(ref)
			if err != nil {
				return uploadBatchSharedPlan{}, err
			}
			return uploadBatchSharedPlan{state: state, target: target, ref: &ref}, nil
		case idle && state.ref != nil:
			state.phase = uploadBatchNewTargetLookingUp
			ref := copyDataSetRef(*state.ref)
			b.mu.Unlock()
			if err := b.waitSharedDataSetVisible(ctx, state, unbound, ref); err != nil {
				return uploadBatchSharedPlan{}, err
			}
		case idle && state.needsLookup:
			state.phase = uploadBatchNewTargetLookingUp
			clientDataSetID := copyBigInt(state.clientDataSetID)
			b.mu.Unlock()
			if err := b.lookupSharedDataSet(ctx, state, unbound, clientDataSetID); err != nil {
				return uploadBatchSharedPlan{}, err
			}
		case idle && create:
			state.phase = uploadBatchNewTargetCreating
			plan := uploadBatchSharedPlan{state: state, clientDataSetID: copyBigInt(state.clientDataSetID)}
			b.mu.Unlock()
			return plan, nil
		case !create && (idle || state.phase == uploadBatchNewTargetCreating):
			plan := uploadBatchSharedPlan{state: state, clientDataSetID: copyBigInt(state.clientDataSetID)}
			b.mu.Unlock()
			return plan, nil
		default:
			changed := state.changed
			b.mu.Unlock()
			select {
			case <-changed:
			case <-ctx.Done():
				return uploadBatchSharedPlan{}, ctx.Err()
			case <-b.ctx.Done():
				return uploadBatchSharedPlan{}, ErrClosed
			}
		}
	}
}

// waitSharedDataSetVisible waits for chain reads to see a newly created data
// set, because add-pieces validates the data set before submitting.
func (b *UploadBatcher) waitSharedDataSetVisible(ctx context.Context, state *uploadBatchNewTarget, unbound uploadBatchUnboundTarget, ref DataSetRef) error {
	deadline := time.Now().Add(b.config.dataSetVisibleTimeout)
	var lastErr error
	for {
		found, ok, err := unbound.findDataSetByClientDataSetID(ctx, ref.ClientDataSetID())
		switch {
		case err == nil && ok:
			b.finishSharedLookup(state, func() {
				state.ref = &found
				state.refVisible = true
			})
			return nil
		case errors.Is(err, ErrUninitialized):
			b.finishSharedLookup(state, func() { state.refVisible = true })
			return nil
		case err != nil && ctx.Err() != nil:
			b.finishSharedLookup(state, func() {})
			return err
		case err != nil:
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			b.finishSharedLookup(state, func() {})
			if lastErr != nil {
				return fmt.Errorf("storage.UploadBatcher: data set %s is not visible: %w: %w", ref.DataSetID().String(), ErrDataSetUnavailable, lastErr)
			}
			return fmt.Errorf("storage.UploadBatcher: data set %s is not visible: %w", ref.DataSetID().String(), ErrDataSetUnavailable)
		}
		timer := time.NewTimer(b.config.dataSetPollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			b.finishSharedLookup(state, func() {})
			return ctx.Err()
		case <-b.ctx.Done():
			timer.Stop()
			b.finishSharedLookup(state, func() {})
			return ErrClosed
		}
	}
}

// lookupSharedDataSet recovers a data set that a failed create may have made.
func (b *UploadBatcher) lookupSharedDataSet(ctx context.Context, state *uploadBatchNewTarget, unbound uploadBatchUnboundTarget, clientDataSetID types.BigInt) error {
	ref, found, err := unbound.findDataSetByClientDataSetID(ctx, clientDataSetID)
	var replacementID types.BigInt
	if errors.Is(err, ErrDataSetCorrelationConflict) {
		var idErr error
		replacementID, idErr = randomClientDataSetID()
		if idErr != nil {
			b.finishSharedLookup(state, func() {})
			return idErr
		}
	}
	var out error
	b.finishSharedLookup(state, func() {
		switch {
		case err == nil && found:
			state.ref = &ref
			state.refVisible = true
			state.needsLookup = false
		case err == nil, errors.Is(err, ErrUninitialized):
			// Without a chain reader, keep the ID so at most one create can succeed.
			state.needsLookup = false
		case errors.Is(err, ErrDataSetCorrelationConflict):
			state.clientDataSetID = replacementID
			state.needsLookup = false
		default:
			out = fmt.Errorf("storage.UploadBatcher: recover shared data set: %w", err)
		}
	})
	return out
}

func (b *UploadBatcher) finishSharedLookup(state *uploadBatchNewTarget, update func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	update()
	state.phase = uploadBatchNewTargetIdle
	state.notifyLocked()
}

func (b *UploadBatcher) markSharedCreateSubmitted(state *uploadBatchNewTarget) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if state.phase == uploadBatchNewTargetCreating {
		state.phase = uploadBatchNewTargetSubmitted
		state.notifyLocked()
	}
}

// finishSharedCreate records the outcome of the creating batch. Any failure may
// have reached the chain, so the next attempt looks the data set up first.
func (b *UploadBatcher) finishSharedCreate(state *uploadBatchNewTarget, clientDataSetID, providerID types.BigInt, result *CommitResult, err error) error {
	if err == nil && (!result.DataSet.valid() ||
		!result.DataSet.ProviderID().Equal(providerID) ||
		!result.DataSet.ClientDataSetID().Equal(clientDataSetID)) {
		err = errors.New("storage.UploadBatcher: create result does not match the shared data set")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	state.phase = uploadBatchNewTargetIdle
	if err == nil {
		ref := copyDataSetRef(result.DataSet)
		state.ref = &ref
		state.refVisible = false
		state.needsLookup = false
	} else {
		state.needsLookup = true
	}
	state.notifyLocked()
	return err
}

// forgetTerminatedSharedDataSet reports whether a batch should retry with a
// replacement for a terminated shared data set.
func (b *UploadBatcher) forgetTerminatedSharedDataSet(state *uploadBatchNewTarget, ref DataSetRef, err error) bool {
	if _, ok := errors.AsType[*DataSetPDPPaymentTerminatedError](err); !ok {
		return false
	}
	clientDataSetID, idErr := randomClientDataSetID()
	if idErr != nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	if state.ref != nil && state.ref.Equal(ref) {
		if state.phase != uploadBatchNewTargetIdle {
			return true
		}
		state.ref = nil
		state.refVisible = false
		state.needsLookup = false
		state.clientDataSetID = clientDataSetID
		state.notifyLocked()
	}
	return true
}

func (b *UploadBatcher) releaseDataSetGate(key string, gate *uploadBatchDataSetGate, acquired bool) {
	if acquired {
		<-gate.slot
	}
	b.mu.Lock()
	gate.refs--
	if gate.refs == 0 && b.datasetGates[key] == gate {
		delete(b.datasetGates, key)
	}
	b.mu.Unlock()
}

func validateUploadBatchSubmissionPieces(pieces []PieceInput, submitted []cid.Cid) error {
	if len(submitted) != len(pieces) {
		return fmt.Errorf("storage.UploadBatcher: submission piece count %d does not match batch count %d", len(submitted), len(pieces))
	}
	for i, piece := range pieces {
		if !submitted[i].Equals(piece.PieceCID) {
			return fmt.Errorf("storage.UploadBatcher: submission piece order does not match batch at index %d", i)
		}
	}
	return nil
}

func uploadBatchGateKey(ref DataSetRef, serviceURL string) string {
	return ref.ProviderID().String() + "\x00" + ref.DataSetID().String() + "\x00" + ref.ClientDataSetID().String() + "\x00" + serviceURL
}

func acquireUploadBatchSlot(ctx context.Context, slot chan struct{}) error {
	select {
	case slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *UploadBatcher) finishFlight(flight *uploadBatchFlight, result *CommitResult, err error) {
	b.mu.Lock()
	if b.closed {
		err = ErrClosed
		result = nil
	}
	for i, slot := range flight.slots {
		if slot.done {
			continue
		}
		var item *CommitResult
		if err == nil {
			copyResult := *result
			copyResult.DataSet = copyDataSetRef(result.DataSet)
			copyResult.PieceIDs = []types.BigInt{copyBigInt(result.PieceIDs[i])}
			item = &copyResult
		}
		slot.done = true
		slot.events <- uploadBatchEvent{result: item, err: err, final: true}
	}
	flight.err = err
	delete(b.flights, flight.seq)
	if err != nil && !b.closed {
		b.failedFlights[flight.seq] = &uploadBatchFailure{
			seq:            flight.seq,
			minReservation: flight.minReservation,
			err:            err,
		}
	}
	close(flight.done)
	b.mu.Unlock()
}

func (b *UploadBatcher) sendSlotEvent(slot *uploadBatchSlot, event uploadBatchEvent) {
	b.mu.Lock()
	if slot.done {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	slot.events <- event
}

func (b *UploadBatcher) observeSlot(slot *uploadBatchSlot) {
	if b == nil || slot == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	slot.observed = true
	flight := slot.flight
	if flight == nil {
		return
	}
	if _, ok := b.failedFlights[flight.seq]; !ok {
		return
	}
	for _, item := range flight.slots {
		if !item.observed {
			return
		}
	}
	delete(b.failedFlights, flight.seq)
}

func (task *uploadBatchTask) wait(ctx context.Context, onSubmitted func(string)) (*CommitResult, error) {
	if task == nil || task.slot == nil {
		return nil, fmt.Errorf("storage.UploadBatcher.wait: %w: nil task", ErrInvalidArgument)
	}
	var batcherDone <-chan struct{}
	if task.batcher != nil {
		batcherDone = task.batcher.ctx.Done()
	}
	observeFinal := func(result *CommitResult, err error) (*CommitResult, error) {
		if task.batcher != nil {
			task.batcher.observeSlot(task.slot)
		}
		return result, err
	}
	handleEvent := func(event uploadBatchEvent, allowCallback bool) (*CommitResult, error, bool) {
		if event.final {
			return event.result, event.err, true
		}
		if allowCallback && event.submission != nil && onSubmitted != nil {
			onSubmitted(event.submission.TransactionID)
		}
		return nil, nil, false
	}
	drainEvents := func(allowCallback bool) (*CommitResult, error, bool) {
		for {
			select {
			case event := <-task.slot.events:
				if result, err, final := handleEvent(event, allowCallback); final {
					return result, err, true
				}
			default:
				return nil, nil, false
			}
		}
	}
	for {
		allowCallback := ctx.Err() == nil && (task.batcher == nil || task.batcher.ctx.Err() == nil)
		if result, err, final := drainEvents(allowCallback); final {
			return observeFinal(result, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, uploadBatchContextError(ctx, err)
		}
		if task.batcher != nil && task.batcher.ctx.Err() != nil {
			return nil, ErrClosed
		}
		select {
		case <-ctx.Done():
			if result, err, final := drainEvents(false); final {
				return observeFinal(result, err)
			}
			return nil, uploadBatchContextError(ctx, ctx.Err())
		case <-batcherDone:
			if result, err, final := drainEvents(false); final {
				return observeFinal(result, err)
			}
			return nil, ErrClosed
		case event := <-task.slot.events:
			allowCallback := ctx.Err() == nil && (task.batcher == nil || task.batcher.ctx.Err() == nil)
			if result, err, final := handleEvent(event, allowCallback); final {
				return observeFinal(result, err)
			}
		}
	}
}

// authorizePull returns the pull target and authorization for a secondary,
// using the shared data set for unbound targets.
func (b *UploadBatcher) authorizePull(ctx context.Context, target StorageContext, pieces []PieceInput) (StorageContext, []byte, error) {
	const op = "storage.UploadBatcher.authorizePull"
	if err := b.validateTarget(target); err != nil {
		return nil, nil, err
	}
	auth := commitAuthorization{
		identity:        b.identity,
		provider:        target.GetProviderInfo(),
		signer:          b.signer,
		dataSetMetadata: target.DataSetMetadata(),
		withCDN:         target.CDNEnabled(),
	}
	if ref, ok := target.DataSetRef(); ok {
		extraData, _, err := presignCommitAuthorization(ctx, op, auth, &ref, pieces, nil)
		if err != nil {
			return nil, nil, err
		}
		return target, extraData, nil
	}
	key, _, err := uploadBatchTargetKey(target)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}
	plan, err := b.resolveSharedDataSet(ctx, key, target.(uploadBatchUnboundTarget), false)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", op, err)
	}
	if plan.ref != nil {
		extraData, _, err := presignCommitAuthorization(ctx, op, auth, plan.ref, pieces, nil)
		if err != nil {
			return nil, nil, err
		}
		return plan.target, extraData, nil
	}
	extraData, _, err := presignCommitAuthorization(ctx, op, auth, nil, pieces, &plan.clientDataSetID)
	if err != nil {
		return nil, nil, err
	}
	return target, extraData, nil
}

// Flush waits for uploads started before this call to reach the batcher,
// submits their windows, and waits for final confirmations. Later compatible
// uploads may join those windows and are not guaranteed to be included or
// excluded. Canceling ctx stops only this wait, does not cancel shared work,
// and does not prevent a later Flush from reporting batch failures.
func (b *UploadBatcher) Flush(ctx context.Context) error {
	if b == nil {
		return fmt.Errorf("storage.UploadBatcher.Flush: %w: nil batcher", ErrInvalidArgument)
	}
	if ctx == nil {
		return fmt.Errorf("storage.UploadBatcher.Flush: %w: nil context", ErrInvalidArgument)
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}
	b.nextFlush++
	flushID := b.nextFlush
	flush := &uploadBatchFlush{
		barrier:  b.nextReservation,
		batches:  make(map[uint64]*uploadBatchFlight),
		failures: make(map[uint64]*uploadBatchFailure),
	}
	for seq, flight := range b.flights {
		if flight.minReservation <= flush.barrier {
			flush.batches[seq] = flight
		}
	}
	for seq, failure := range b.failedFlights {
		if failure.minReservation <= flush.barrier {
			flush.failures[seq] = failure
		}
	}
	b.flushes[flushID] = flush
	reservations := make([]chan struct{}, 0, len(b.reservations))
	for seq, done := range b.reservations {
		if seq <= flush.barrier {
			reservations = append(reservations, done)
		}
	}
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.flushes, flushID)
		b.mu.Unlock()
	}()
	for _, done := range reservations {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		case <-b.ctx.Done():
			return ErrClosed
		}
	}
	b.mu.Lock()
	var launches []*uploadBatchFlight
	windows := make([]*uploadBatchWindow, 0, len(b.windows))
	for _, window := range b.windows {
		for _, slot := range window.slots {
			if slot.reservation <= flush.barrier {
				windows = append(windows, window)
				break
			}
		}
	}
	sort.Slice(windows, func(i, j int) bool {
		left := windowMinReservation(windows[i])
		right := windowMinReservation(windows[j])
		if left != right {
			return left < right
		}
		return windows[i].key < windows[j].key
	})
	for _, window := range windows {
		launches = append(launches, b.sealWindowLocked(window))
	}
	batches := make([]*uploadBatchFlight, 0, len(flush.batches))
	for _, flight := range flush.batches {
		batches = append(batches, flight)
	}
	failures := make([]*uploadBatchFailure, 0, len(flush.failures))
	for _, failure := range flush.failures {
		failures = append(failures, failure)
	}
	b.mu.Unlock()
	b.launchFlights(launches)
	sort.Slice(batches, func(i, j int) bool { return batches[i].seq < batches[j].seq })
	sort.Slice(failures, func(i, j int) bool { return failures[i].seq < failures[j].seq })
	var joined []error
	for _, flight := range batches {
		select {
		case <-flight.done:
			if flight.err != nil {
				joined = append(joined, flight.err)
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-b.ctx.Done():
			return ErrClosed
		}
	}
	for _, failure := range failures {
		if failure.err != nil {
			joined = append(joined, failure.err)
		}
	}
	b.mu.Lock()
	for _, flight := range batches {
		delete(b.failedFlights, flight.seq)
	}
	for _, failure := range failures {
		delete(b.failedFlights, failure.seq)
	}
	b.mu.Unlock()
	return errors.Join(joined...)
}

// Close aborts open and in-flight batches. It is safe to call concurrently and
// does not wait for external signers or provider requests to return. Call
// [UploadBatcher.Flush] first when pending uploads must complete.
func (b *UploadBatcher) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.cancel(ErrClosed)
	boundContexts := make([]context.CancelCauseFunc, 0, len(b.boundContexts))
	for id, cancel := range b.boundContexts {
		boundContexts = append(boundContexts, cancel)
		delete(b.boundContexts, id)
	}
	var slots []*uploadBatchSlot
	for _, window := range b.windows {
		if window.timer != nil {
			window.timer.Stop()
		}
		for _, slot := range window.slots {
			if !slot.done {
				slot.done = true
				slots = append(slots, slot)
			}
		}
	}
	for _, flight := range b.flights {
		for _, slot := range flight.slots {
			if !slot.done {
				slot.done = true
				slots = append(slots, slot)
			}
		}
	}
	b.windows = make(map[string]*uploadBatchWindow)
	b.failedFlights = make(map[uint64]*uploadBatchFailure)
	b.newTargets = make(map[string]*uploadBatchNewTarget)
	b.transfers = make(map[string]int)
	for _, slot := range slots {
		slot.events <- uploadBatchEvent{err: ErrClosed, final: true}
	}
	b.mu.Unlock()
	for _, cancel := range boundContexts {
		cancel(ErrClosed)
	}
	return nil
}
