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
	defaultUploadMaxWait                  = 30 * time.Second
	defaultUploadMaxConcurrentSubmissions = 4
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
}

// UploadBatcherOption configures upload batching behavior.
type UploadBatcherOption func(*uploadBatcherConfig)

// WithUploadIdleWait sets the inactivity delay before an open window is
// submitted. A zero duration submits as soon as a piece is ready.
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

// WithUploadMaxWait sets the maximum age of an open window. A zero duration
// submits as soon as the first piece is ready.
func WithUploadMaxWait(wait time.Duration) UploadBatcherOption {
	return func(cfg *uploadBatcherConfig) {
		cfg.maxWait = wait
		cfg.maxWaitEnabled = true
	}
}

// WithoutUploadMaxWait disables age-based submission.
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
	submissionSlots chan struct{}
}

type uploadBatchDataSetGate struct {
	slot chan struct{}
	refs int
}

type uploadBatchWindow struct {
	key             string
	target          StorageContext
	ref             *DataSetRef
	clientDataSetID *types.BigInt
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
	seq             uint64
	minReservation  uint64
	target          StorageContext
	ref             *DataSetRef
	clientDataSetID *types.BigInt
	slots           []*uploadBatchSlot
	done            chan struct{}
	err             error
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
}

// NewUploadBatcher creates an independent batching coordinator. By default,
// an open window is submitted after three seconds of inactivity or 30 seconds
// from its first ready piece, and up to four batches may be signed and
// submitted concurrently. When options of the same kind are repeated, the
// last one takes effect.
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
		maxWait:                  defaultUploadMaxWait,
		maxWaitEnabled:           true,
		maxConcurrentSubmissions: defaultUploadMaxConcurrentSubmissions,
		clock:                    systemUploadBatchClock{},
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
		r.batcher.mu.Lock()
		if done, ok := r.batcher.reservations[r.seq]; ok {
			delete(r.batcher.reservations, r.seq)
			close(done)
		}
		r.batcher.mu.Unlock()
	})
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
	return nil
}

func (b *UploadBatcher) enqueue(ctx context.Context, reservation uint64, target StorageContext, piece PieceInput) (*uploadBatchTask, error) {
	const op = "storage.UploadBatcher.enqueue"
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
	var candidateID *types.BigInt
	if ref == nil {
		id, idErr := clientDataSetIDOrRandom(nil)
		if idErr != nil {
			return nil, fmt.Errorf("%s: %w", op, idErr)
		}
		candidateID = &id
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
	window := b.windows[key]
	if window != nil {
		pieces := append(windowPieces(window), clonePieceInput(piece))
		_, duplicate := window.pieceCIDs[canonicalCommitPieceCIDKey(piece.PieceCID)]
		if duplicate || b.validateCandidate(window.target, window.ref, window.clientDataSetID, pieces) != nil {
			launches = append(launches, b.sealWindowLocked(window))
			window = nil
		}
	}
	if window == nil {
		window = &uploadBatchWindow{
			key:             key,
			target:          target,
			ref:             copyDataSetRefPtr(ref),
			clientDataSetID: copyBigIntPtr(candidateID),
			pieceCIDs:       make(map[string]struct{}),
		}
		if err := b.validateCandidate(target, window.ref, window.clientDataSetID, []PieceInput{piece}); err != nil {
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
	immediate := b.config.idleWaitEnabled && b.config.idleWait == 0 ||
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
	var deadline time.Time
	if b.config.idleWaitEnabled {
		deadline = window.idleDeadline
	}
	if b.config.maxWaitEnabled && (deadline.IsZero() || window.maxDeadline.Before(deadline)) {
		deadline = window.maxDeadline
	}
	if deadline.IsZero() {
		return
	}
	window.timerGeneration++
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
		seq:             b.nextBatch,
		minReservation:  window.slots[0].reservation,
		target:          window.target,
		ref:             copyDataSetRefPtr(window.ref),
		clientDataSetID: copyBigIntPtr(window.clientDataSetID),
		slots:           append([]*uploadBatchSlot(nil), window.slots...),
		done:            make(chan struct{}),
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
	var gate *uploadBatchDataSetGate
	var gateKey string
	if flight.ref != nil {
		gateKey = flight.targetKey()
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
			b.finishFlight(flight, nil, err)
			return
		}
	}
	if err := acquireUploadBatchSlot(b.ctx, b.submissionSlots); err != nil {
		if gate != nil {
			b.releaseDataSetGate(gateKey, gate, true)
		}
		b.finishFlight(flight, nil, err)
		return
	}
	extraData, _, err := presignCommitAuthorization(b.ctx, "storage.UploadBatcher", commitAuthorization{
		identity:        b.identity,
		provider:        flight.target.GetProviderInfo(),
		signer:          b.signer,
		dataSetMetadata: flight.target.DataSetMetadata(),
		withCDN:         flight.target.CDNEnabled(),
	}, flight.ref, pieces, flight.clientDataSetID)
	if err == nil {
		err = b.ctx.Err()
	}
	var submission *CommitSubmission
	if err == nil {
		submission, err = flight.target.SubmitCommit(b.ctx, CommitRequest{
			Pieces:          pieces,
			ExtraData:       extraData,
			ClientDataSetID: copyBigIntPtr(flight.clientDataSetID),
		})
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
		b.finishFlight(flight, nil, err)
		return
	}
	if submission == nil {
		b.finishFlight(flight, nil, errors.New("storage.UploadBatcher: submit commit returned nil submission"))
		return
	}
	if err := validateUploadBatchSubmissionPieces(pieces, submission.PieceCIDs); err != nil {
		b.finishFlight(flight, nil, err)
		return
	}
	for _, slot := range flight.slots {
		copySubmission := copyCommitSubmission(*submission)
		b.sendSlotEvent(slot, uploadBatchEvent{submission: &copySubmission})
	}
	result, err := flight.target.WaitForCommit(b.ctx, *submission)
	err = uploadBatchContextError(b.ctx, err)
	if err == nil {
		if result == nil {
			err = errors.New("storage.UploadBatcher: wait for commit returned nil result")
		} else {
			err = validateConfirmedPieceIDs(result.PieceIDs, len(flight.slots))
		}
	}
	b.finishFlight(flight, result, err)
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

func (f *uploadBatchFlight) targetKey() string {
	return f.ref.ProviderID().String() + "\x00" + f.ref.DataSetID().String() + "\x00" + f.ref.ClientDataSetID().String() + "\x00" + f.target.ServiceURL()
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

func (b *UploadBatcher) presignExisting(ctx context.Context, target StorageContext, pieces []PieceInput) ([]byte, error) {
	if err := b.validateTarget(target); err != nil {
		return nil, err
	}
	ref, ok := target.DataSetRef()
	if !ok {
		return nil, fmt.Errorf("storage.UploadBatcher.presignExisting: %w: unbound context", ErrInvalidArgument)
	}
	extraData, _, err := presignCommitAuthorization(ctx, "storage.UploadBatcher.presignExisting", commitAuthorization{
		identity: b.identity,
		provider: target.GetProviderInfo(),
		signer:   b.signer,
	}, &ref, pieces, nil)
	return extraData, err
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
	for _, slot := range slots {
		slot.events <- uploadBatchEvent{err: ErrClosed, final: true}
	}
	b.mu.Unlock()
	for _, cancel := range boundContexts {
		cancel(ErrClosed)
	}
	return nil
}
