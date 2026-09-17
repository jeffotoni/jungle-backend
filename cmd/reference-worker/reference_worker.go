package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend/cmd/reference-worker/config"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend/internal/domain/wager"
)

const pendingReferenceFailureCode = "REFERENCE_NOT_FOUND"

type ReferenceWorker struct {
	tx           ports.TxManager
	pending      ports.PendingReferenceStore
	wagers       *appwager.Service
	logger       *log.Logger
	pollInterval time.Duration
	batchSize    int
	referenceTTL time.Duration
	maxAttempts  int
	retryBase    time.Duration
	retryMax     time.Duration
	cancel       context.CancelFunc
	done         chan struct{}
}

type referenceOutcome struct {
	transactionID string
	status        wager.Status
	attempts      int
	nextAttemptAt time.Time
	code          string
}

func NewReferenceWorker(
	lc fx.Lifecycle,
	tx ports.TxManager,
	pending ports.PendingReferenceStore,
	wagers *appwager.Service,
	logger *log.Logger,
) *ReferenceWorker {
	worker := &ReferenceWorker{
		tx:           tx,
		pending:      pending,
		wagers:       wagers,
		logger:       logger,
		pollInterval: config.POLL_INTERVAL,
		batchSize:    config.BATCH_SIZE,
		referenceTTL: config.REFERENCE_TTL,
		maxAttempts:  config.REFERENCE_MAX_ATTEMPTS,
		retryBase:    config.RETRY_BASE,
		retryMax:     config.RETRY_MAX,
		done:         make(chan struct{}),
	}
	worker.normalizeConfig()

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			workerContext, cancel := context.WithCancel(context.Background())
			worker.cancel = cancel
			_ = worker.logger.Info().
				Component("reference-worker").
				Action("startup").
				Int("batchSize", worker.batchSize).
				Str("pollInterval", worker.pollInterval.String()).
				Str("referenceTTL", worker.referenceTTL.String()).
				Int("maxAttempts", worker.maxAttempts).
				Msg("reference worker started").
				Send()
			go worker.run(workerContext)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if worker.cancel != nil {
				worker.cancel()
			}
			select {
			case <-worker.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	return worker
}

func (w *ReferenceWorker) normalizeConfig() {
	if w.pollInterval <= 0 {
		w.pollInterval = time.Second
	}
	if w.batchSize <= 0 {
		w.batchSize = 10
	}
	if w.referenceTTL <= 0 {
		w.referenceTTL = 15 * time.Minute
	}
	if w.maxAttempts <= 0 {
		w.maxAttempts = 10
	}
	if w.retryBase <= 0 {
		w.retryBase = time.Second
	}
	if w.retryMax < w.retryBase {
		w.retryMax = w.retryBase
	}
}

func (w *ReferenceWorker) run(ctx context.Context) {
	defer close(w.done)
	for {
		if ctx.Err() != nil {
			return
		}
		count, err := w.processBatch(ctx)
		if err != nil {
			w.logError(ctx, err)
			if !waitForRetry(ctx, w.retryBase) {
				return
			}
			continue
		}
		if count == 0 && !waitForRetry(ctx, w.pollInterval) {
			return
		}
	}
}

func (w *ReferenceWorker) processBatch(ctx context.Context) (int, error) {
	var count int
	var outcomes []referenceOutcome
	err := w.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		records, err := w.pending.ClaimPendingReferences(ctx, tx, w.batchSize)
		if err != nil {
			return err
		}
		count = len(records)
		outcomes = make([]referenceOutcome, 0, len(records))
		for _, record := range records {
			outcome, err := w.processRecord(ctx, tx, record)
			if err != nil {
				return err
			}
			outcomes = append(outcomes, outcome)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, outcome := range outcomes {
		w.logOutcome(ctx, outcome)
	}
	return count, nil
}

func (w *ReferenceWorker) processRecord(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
) (referenceOutcome, error) {
	result, err := w.wagers.ResumePendingReference(ctx, tx, record)
	if err != nil {
		return referenceOutcome{}, err
	}
	if result.Status != wager.StatusPendingReference {
		return referenceOutcome{
			transactionID: result.TransactionID,
			status:        result.Status,
			code:          result.FailureCode,
		}, nil
	}

	attempts := record.ReferenceAttempts + 1
	now := time.Now().UTC()
	if attempts >= w.maxAttempts || w.referenceExpired(record, now) {
		if err := w.pending.UpdatePendingReferenceRetry(ctx, tx, record.ID, attempts, now); err != nil {
			return referenceOutcome{}, err
		}
		result, err = w.wagers.RejectPendingReference(ctx, tx, record, pendingReferenceFailureCode)
		if err != nil {
			return referenceOutcome{}, err
		}
		return referenceOutcome{
			transactionID: result.TransactionID,
			status:        result.Status,
			attempts:      attempts,
			code:          result.FailureCode,
		}, nil
	}

	nextAttemptAt := now.Add(referenceRetryDelay(w.retryBase, w.retryMax, attempts))
	if err := w.pending.UpdatePendingReferenceRetry(ctx, tx, record.ID, attempts, nextAttemptAt); err != nil {
		return referenceOutcome{}, err
	}
	return referenceOutcome{
		transactionID: record.ID,
		status:        wager.StatusPendingReference,
		attempts:      attempts,
		nextAttemptAt: nextAttemptAt,
	}, nil
}

func (w *ReferenceWorker) referenceExpired(record ports.WagerRecord, now time.Time) bool {
	pendingAt := record.ReferencePendingAt
	if pendingAt == nil || pendingAt.IsZero() {
		pendingAt = &record.CreatedAt
	}
	return !now.Before(pendingAt.Add(w.referenceTTL))
}

func (w *ReferenceWorker) logOutcome(ctx context.Context, outcome referenceOutcome) {
	entry := w.logger.Info().
		Ctx(ctx).
		Component("reference-worker").
		Action("process").
		Str("transactionId", outcome.transactionID).
		Str("status", string(outcome.status)).
		Int("attempts", outcome.attempts)
	if outcome.status == wager.StatusPendingReference {
		_ = entry.
			Action("retry").
			Str("nextAttemptAt", outcome.nextAttemptAt.Format(time.RFC3339Nano)).
			Msg("pending reference will be retried").
			Send()
		return
	}
	if outcome.code != "" {
		entry.Str("failureCode", outcome.code)
	}
	_ = entry.Msg("pending reference processed").Send()
}

func (w *ReferenceWorker) logError(ctx context.Context, err error) {
	_ = w.logger.Error().
		Ctx(ctx).
		Component("reference-worker").
		Action("process").
		Err("error", err).
		Msg("reference worker operation failed").
		Send()
}

func referenceRetryDelay(base, maximum time.Duration, attempts int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maximum < base {
		maximum = base
	}
	if attempts <= 1 {
		return base
	}
	if attempts > 31 {
		attempts = 31
	}
	delay := base
	for index := 1; index < attempts && delay < maximum; index++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
