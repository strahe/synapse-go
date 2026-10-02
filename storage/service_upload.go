package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/redact"
	"github.com/strahe/synapse-go/types"
)

// Each slot has one pull producer. Admission freezes its target and authorization;
// a commit worker writes the outcome. Aggregation runs after both have joined.
type uploadCopy struct {
	target         StorageContext
	role           CopyRole
	extraData      []byte
	ready          bool
	failedAttempts []FailedAttempt
	result         *CommitResult
	submission     *CommitSubmission
	err            error
}

type uploadPipeline struct {
	service          *Service
	ctx              context.Context
	op               string
	opts             *UploadOptions
	storeResult      *StoreResult
	pieces           []PieceInput
	copies           []uploadCopy
	allowReplacement bool
	reservation      *uploadReservation
	usedProviders    map[string]types.BigInt
	replacementGate  chan struct{}
	commitJobs       chan int
	commitWG         sync.WaitGroup
}

func (s *Service) uploadWithContexts(ctx context.Context, op string, r io.Reader, contexts []StorageContext, opts *UploadOptions, requestedCopies int, allowReplacement bool, reservation *uploadReservation) (*UploadResult, error) {
	ctx, guard := newUploadCallbackGuard(ctx, op, s.logger)
	defer guard.rethrow()
	opts = guard.wrapUploadOptions(opts)
	if s.uploadBatcher != nil {
		for _, target := range contexts {
			if err := s.uploadBatcher.validateTarget(target); err != nil {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
		}
	}
	primary := contexts[0]
	var transfer *uploadBatchTransfer
	if s.uploadBatcher != nil {
		var err error
		transfer, err = reservation.beginTransfer(primary)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}
	storeOpts := &StoreOptions{}
	if opts != nil {
		storeOpts.PieceCID = opts.PieceCID
		storeOpts.OnProgress = opts.OnProgress
	}
	stored, err := primary.Store(ctx, r, storeOpts)
	if err != nil {
		transfer.end()
		return nil, &StoreError{
			ProviderID: primary.ProviderID(),
			Endpoint:   redact.URLString(primary.ServiceURL()),
			Cause:      uploadBatchContextError(ctx, err),
		}
	}
	if opts != nil && opts.OnStored != nil {
		opts.OnStored(primary.ProviderID(), stored.PieceCID)
	}
	p := &uploadPipeline{
		service: s, ctx: ctx, op: op, opts: opts, storeResult: stored,
		pieces:           []PieceInput{{PieceCID: stored.PieceCID, PieceMetadata: cloneMetadata(opts)}},
		copies:           make([]uploadCopy, len(contexts)),
		allowReplacement: allowReplacement,
		reservation:      reservation,
		usedProviders:    make(map[string]types.BigInt, len(contexts)),
		replacementGate:  make(chan struct{}, 1),
	}
	for i, target := range contexts {
		p.copies[i] = uploadCopy{target: target, role: CopyRoleSecondary}
		id := target.ProviderID()
		p.usedProviders[idconv.Key(id)] = copyBigInt(id)
	}
	p.copies[0].role = CopyRolePrimary
	if s.uploadBatcher == nil {
		p.commitJobs = make(chan int, len(contexts))
		for range min(s.commitConcurrency, len(contexts)) {
			p.commitWG.Go(func() {
				for index := range p.commitJobs {
					p.commitCopy(index)
				}
			})
		}
	}
	p.admitCopy(0, nil, transfer)
	p.pullSecondaries()
	// Flush waits for producers to release their reservation. Confirmation
	// waiters must not hold that reservation while waiting for Flush to submit.
	reservation.release()
	if p.commitJobs != nil {
		close(p.commitJobs)
	}
	p.commitWG.Wait()
	return p.result(requestedCopies)
}

func (p *uploadPipeline) pullSecondaries() {
	jobs := make(chan int, len(p.copies)-1)
	for i := 1; i < len(p.copies); i++ {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for range min(p.service.pullConcurrency, len(p.copies)-1) {
		workers.Go(func() {
			for index := range jobs {
				if p.ctx.Err() != nil {
					return
				}
				p.pullSecondary(index)
			}
		})
	}
	workers.Wait()
}

func (p *uploadPipeline) pullSecondary(index int) {
	slot := &p.copies[index]
	attemptsUsed := 1
	for p.ctx.Err() == nil {
		if p.pullAttempt(index) || !p.allowReplacement || p.ctx.Err() != nil {
			return
		}
		found := false
		for attemptsUsed < p.service.maxSecondaryAttempts && p.ctx.Err() == nil {
			selection, err := p.selectReplacement()
			if err != nil {
				return
			}
			attemptsUsed++
			if selection.err == nil {
				selection.err = p.service.validateUploadContextsWritable(p.ctx, []StorageContext{selection.target})
			}
			if selection.err != nil {
				var providerID types.BigInt
				if !isNilStorageContext(selection.target) {
					providerID = selection.target.ProviderID()
				}
				p.recordPullFailure(slot, providerID, CopyStagePresign, selection.err)
				continue
			}
			slot.target = selection.target
			found = true
			break
		}
		if !found {
			return
		}
	}
}

type uploadReplacement struct {
	target StorageContext
	err    error
}

func (p *uploadPipeline) selectReplacement() (uploadReplacement, error) {
	select {
	case p.replacementGate <- struct{}{}:
	case <-p.ctx.Done():
		return uploadReplacement{}, uploadBatchContextError(p.ctx, p.ctx.Err())
	}
	defer func() { <-p.replacementGate }()
	if err := p.ctx.Err(); err != nil {
		return uploadReplacement{}, uploadBatchContextError(p.ctx, err)
	}
	target, err := p.service.resolver.SelectReplacement(p.ctx, selectProviderContextOptionsForReplacement(p.opts, p.usedProviders))
	if err != nil {
		return uploadReplacement{}, err
	}
	if err := p.ctx.Err(); err != nil {
		return uploadReplacement{}, uploadBatchContextError(p.ctx, err)
	}
	var excluded []types.BigInt
	if p.opts != nil {
		excluded = p.opts.ExcludeProviderIDs
	}
	validationErr := p.service.validateUploadReplacement(p.op, target, p.usedProviders, excluded)
	if !isNilStorageContext(target) {
		id := target.ProviderID()
		if !id.IsZero() {
			p.usedProviders[idconv.Key(id)] = copyBigInt(id)
		}
	}
	return uploadReplacement{target: target, err: validationErr}, nil
}

func (p *uploadPipeline) pullAttempt(index int) bool {
	slot := &p.copies[index]
	target := slot.target
	pullTarget := target
	var transfer *uploadBatchTransfer
	var extraData []byte
	var err error
	if p.service.uploadBatcher != nil {
		transfer, err = p.reservation.beginTransfer(target)
		if err == nil {
			pullTarget, extraData, err = p.service.uploadBatcher.authorizePull(p.ctx, target, p.pieces)
		}
	} else {
		extraData, err = target.presignForCommit(p.ctx, p.pieces)
	}
	defer transfer.end()
	if err != nil {
		p.recordPullFailure(slot, target.ProviderID(), CopyStagePresign, err)
		return false
	}
	if err := p.ctx.Err(); err != nil {
		p.recordPullFailure(slot, target.ProviderID(), CopyStagePull, err)
		return false
	}
	var onProgress func(cid.Cid, PullStatus)
	if p.opts != nil && p.opts.OnPullProgress != nil {
		onProgress = func(pieceCID cid.Cid, status PullStatus) {
			p.opts.OnPullProgress(target.ProviderID(), pieceCID, status)
		}
	}
	result, err := pullTarget.pull(p.ctx, PullRequest{
		Pieces: []cid.Cid{p.storeResult.PieceCID}, From: p.copies[0].target.PieceURL,
		ExtraData: extraData, OnProgress: onProgress,
	})
	if err == nil {
		switch {
		case result == nil:
			err = errors.New("pull returned nil result")
		case result.Status != PullStatusComplete:
			err = fmt.Errorf("pull status %s", result.Status)
		}
	}
	if err != nil {
		if p.ctx.Err() == nil && p.opts != nil && p.opts.OnCopyFailed != nil {
			p.opts.OnCopyFailed(target.ProviderID(), p.storeResult.PieceCID, err)
		}
		p.recordPullFailure(slot, target.ProviderID(), CopyStagePull, err)
		return false
	}
	if p.ctx.Err() == nil && p.opts != nil && p.opts.OnCopyComplete != nil {
		p.opts.OnCopyComplete(target.ProviderID(), p.storeResult.PieceCID)
	}
	p.admitCopy(index, extraData, transfer)
	return true
}

func (p *uploadPipeline) recordPullFailure(slot *uploadCopy, providerID types.BigInt, stage CopyStage, err error) {
	slot.failedAttempts = append(slot.failedAttempts, FailedAttempt{
		ProviderID: providerID, Role: CopyRoleSecondary, Stage: stage,
		Err: uploadBatchContextError(p.ctx, err), Explicit: !p.allowReplacement,
	})
}

func (p *uploadPipeline) admitCopy(index int, extraData []byte, transfer *uploadBatchTransfer) {
	defer transfer.end()
	slot := &p.copies[index]
	slot.ready = true
	if p.service.uploadBatcher == nil {
		slot.extraData = append([]byte(nil), extraData...)
		p.commitJobs <- index
		return
	}
	task, err := p.service.uploadBatcher.enqueue(p.ctx, p.reservation.seq, slot.target, p.pieces[0], transfer)
	if err != nil {
		slot.err = uploadBatchContextError(p.ctx, err)
		return
	}
	p.commitWG.Go(func() {
		var onSubmitted func(string)
		if p.opts != nil && p.opts.OnPiecesAdded != nil {
			onSubmitted = func(txHash string) { p.onPiecesAdded(slot, txHash) }
		}
		slot.result, slot.submission, slot.err = task.wait(p.ctx, onSubmitted)
	})
}

func (p *uploadPipeline) onPiecesAdded(slot *uploadCopy, txHash string) {
	p.opts.OnPiecesAdded(txHash, slot.target.ProviderID(), []SubmittedPiece{{PieceCID: p.storeResult.PieceCID}})
}

func (p *uploadPipeline) commitCopy(index int) {
	slot := &p.copies[index]
	if err := p.ctx.Err(); err != nil {
		slot.err = uploadBatchContextError(p.ctx, err)
		return
	}
	var onSubmitted func(CommitSubmission)
	if p.opts != nil && p.opts.OnPiecesAdded != nil {
		onSubmitted = func(submission CommitSubmission) { p.onPiecesAdded(slot, submission.TransactionID) }
	}
	slot.submission, slot.err = slot.target.submitCommit(p.ctx, commitRequest{CommitRequest: CommitRequest{
		Pieces: p.pieces, ExtraData: slot.extraData, OnSubmitted: onSubmitted,
	}})
	if slot.err == nil && slot.submission == nil {
		slot.err = errors.New("submit commit returned nil submission")
	}
	if slot.err == nil {
		slot.result, slot.err = slot.target.waitForCommit(p.ctx, *slot.submission)
	}
	if slot.err != nil {
		slot.err = uploadBatchContextError(p.ctx, slot.err)
	}
}

func (p *uploadPipeline) result(requestedCopies int) (*UploadResult, error) {
	var failedAttempts []FailedAttempt
	for _, slot := range p.copies {
		failedAttempts = append(failedAttempts, slot.failedAttempts...)
	}
	copies := make([]CopyResult, 0, len(p.copies))
	var primaryCommitErr error
	for _, slot := range p.copies {
		if !slot.ready {
			continue
		}
		err := slot.err
		if err == nil {
			switch {
			case slot.result == nil:
				err = errors.New("commit result missing confirmed identifiers: nil result")
			case !slot.result.DataSet.valid():
				err = errors.New("commit result missing confirmed identifiers: zero dataSetID")
			case !slot.result.DataSet.ProviderID().Equal(slot.target.ProviderID()):
				err = errors.New("commit result data-set provider does not match target context")
			default:
				err = validateConfirmedPieceIDs(slot.result.PieceIDs, len(p.pieces))
			}
		}
		if err != nil {
			if slot.role == CopyRolePrimary {
				primaryCommitErr = err
			}
			failedAttempts = append(failedAttempts, FailedAttempt{
				ProviderID: slot.target.ProviderID(), Role: slot.role, Stage: CopyStageCommit,
				Err: err, Explicit: !p.allowReplacement, Submission: slot.submission,
			})
			continue
		}
		if p.opts != nil && p.opts.OnPiecesConfirmed != nil && p.ctx.Err() == nil {
			confirmed := make([]ConfirmedPiece, len(slot.result.PieceIDs))
			for j, id := range slot.result.PieceIDs {
				confirmed[j] = ConfirmedPiece{PieceID: id, PieceCID: p.storeResult.PieceCID}
			}
			p.opts.OnPiecesConfirmed(slot.result.DataSet.DataSetID(), slot.target.ProviderID(), confirmed)
		}
		copies = append(copies, CopyResult{
			ProviderID: slot.target.ProviderID(), DataSetID: slot.result.DataSet.DataSetID(),
			PieceID: slot.result.PieceIDs[0], Role: slot.role,
			RetrievalURL: slot.target.PieceURL(p.storeResult.PieceCID), IsNewDataSet: slot.result.IsNewDataSet,
		})
	}
	if len(copies) == 0 {
		primary := p.copies[0].target
		return nil, &CommitError{
			ProviderID: primary.ProviderID(), Endpoint: redact.URLString(primary.ServiceURL()),
			Cause: primaryCommitErr, PieceCID: p.storeResult.PieceCID,
			Size: p.storeResult.Size, FailedAttempts: failedAttempts,
		}
	}
	return &UploadResult{
		PieceCID: p.storeResult.PieceCID, Size: p.storeResult.Size, RequestedCopies: requestedCopies,
		Complete: len(copies) >= requestedCopies, Copies: copies, FailedAttempts: failedAttempts,
	}, nil
}
