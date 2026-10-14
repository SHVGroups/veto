// Package worker runs Veto's background jobs: expiring held actions,
// reminders, webhook delivery, ledger checkpoints and verification, and data
// retention. Jobs that must run on one replica at a time take a Postgres
// advisory lock for each run; webhook delivery uses row leases instead, so
// every replica can share the load.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shvgroups/veto/server/internal/app"
	"github.com/shvgroups/veto/server/internal/metrics"
	"github.com/shvgroups/veto/server/internal/notify"
	"github.com/shvgroups/veto/server/internal/store"
)

// Advisory lock keys, one per singleton job.
const (
	lockExpire     int64 = 0x7665746f00000010
	lockRemind     int64 = 0x7665746f00000011
	lockCheckpoint int64 = 0x7665746f00000012
	lockRetention  int64 = 0x7665746f00000013
	lockVerify     int64 = 0x7665746f00000014
)

// Intervals controls how often each job runs.
type Intervals struct {
	Expire     time.Duration
	Remind     time.Duration
	Deliver    time.Duration
	Checkpoint time.Duration
	Retention  time.Duration
	Verify     time.Duration
}

// DefaultIntervals suit production.
func DefaultIntervals() Intervals {
	return Intervals{
		Expire:     5 * time.Second,
		Remind:     30 * time.Second,
		Deliver:    2 * time.Second,
		Checkpoint: 15 * time.Minute,
		Retention:  time.Hour,
		Verify:     24 * time.Hour,
	}
}

// Worker runs the background jobs.
type Worker struct {
	app       *app.App
	store     *store.Store
	sender    *notify.Sender
	log       *slog.Logger
	intervals Intervals
	runs      *metrics.CounterVec
	delivered *metrics.CounterVec
}

// New returns a worker. reg may be nil.
func New(a *app.App, s *store.Store, sender *notify.Sender, log *slog.Logger, iv Intervals, reg *metrics.Registry) *Worker {
	if reg == nil {
		reg = metrics.NewRegistry()
	}
	return &Worker{
		app: a, store: s, sender: sender, log: log, intervals: iv,
		runs:      reg.Counter("veto_worker_runs_total", "Background job runs, by job and result.", "job", "result"),
		delivered: reg.Counter("veto_webhook_deliveries_total", "Webhook delivery attempts, by result.", "result"),
	}
}

// Run starts every job and blocks until the context ends.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	start := func(name string, every time.Duration, lock int64, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, name, every, lock, fn)
		}()
	}
	start("expire", w.intervals.Expire, lockExpire, w.expire)
	start("remind", w.intervals.Remind, lockRemind, w.remind)
	start("deliver", w.intervals.Deliver, 0, w.deliver)
	start("checkpoint", w.intervals.Checkpoint, lockCheckpoint, w.checkpoint)
	start("retention", w.intervals.Retention, lockRetention, w.retention)
	start("verify", w.intervals.Verify, lockVerify, w.verify)
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, name string, every time.Duration, lock int64, fn func(context.Context) error) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		w.runOnce(ctx, name, lock, fn)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (w *Worker) runOnce(ctx context.Context, name string, lock int64, fn func(context.Context) error) {
	defer func() {
		if r := recover(); r != nil {
			w.log.Error("background job panicked", "job", name, "panic", r)
			w.runs.Inc(name, "panic")
		}
	}()
	if lock != 0 {
		release, ok, err := w.store.TryAdvisoryLock(ctx, lock)
		if err != nil {
			if ctx.Err() == nil {
				w.log.Warn("could not take job lock", "job", name, "error", err)
			}
			return
		}
		if !ok {
			w.runs.Inc(name, "skipped")
			return
		}
		defer release()
	}
	if err := fn(ctx); err != nil && ctx.Err() == nil {
		w.log.Error("background job failed", "job", name, "error", err)
		w.runs.Inc(name, "error")
		return
	}
	w.runs.Inc(name, "ok")
}

func (w *Worker) expire(ctx context.Context) error {
	refs, err := w.store.ExpiredPending(ctx, 200)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := w.app.Expire(ctx, ref); err != nil {
			w.log.Error("could not expire action", "action", ref.ActionID, "error", err)
		}
	}
	return nil
}

func (w *Worker) remind(ctx context.Context) error {
	refs, err := w.store.PendingNeedingReminder(ctx, 200)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := w.app.Remind(ctx, ref); err != nil {
			w.log.Error("could not send reminder", "action", ref.ActionID, "error", err)
		}
	}
	return nil
}

func (w *Worker) deliver(ctx context.Context) error {
	due, err := w.store.LeaseDueDeliveries(ctx, 50, 2*time.Minute)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, d := range due {
		wg.Add(1)
		sem <- struct{}{}
		go func(d store.DueDelivery) {
			defer wg.Done()
			defer func() { <-sem }()
			w.sendOne(ctx, d)
		}(d)
	}
	wg.Wait()
	return nil
}

func (w *Worker) sendOne(ctx context.Context, d store.DueDelivery) {
	upd := d.Delivery
	upd.Attempts++
	if d.Disabled {
		upd.Status, upd.LastError = "failed", "webhook is disabled"
		w.finish(ctx, upd, "disabled")
		return
	}
	secret, err := w.app.Keyring.Open(d.SecretEnc, "webhook:"+d.WebhookID)
	if err != nil {
		upd.Status, upd.LastError = "failed", "webhook secret cannot be opened with the current master key"
		w.finish(ctx, upd, "error")
		return
	}
	sctx, cancel := context.WithTimeout(ctx, notify.DeliveryTimeout)
	status, err := w.sender.Send(sctx, d.URL, d.ID, d.Event, secret, d.Payload)
	cancel()
	if status != 0 {
		upd.ResponseStatus = &status
	}
	if err == nil {
		now := time.Now()
		upd.Status, upd.LastError, upd.DeliveredAt = "delivered", "", &now
		w.finish(ctx, upd, "delivered")
		return
	}
	upd.LastError = err.Error()
	if len(upd.LastError) > 500 {
		upd.LastError = upd.LastError[:500]
	}
	if upd.Attempts >= notify.MaxAttempts {
		upd.Status = "failed"
		w.finish(ctx, upd, "failed")
		return
	}
	upd.Status = "pending"
	upd.NextAttemptAt = time.Now().Add(notify.RetryDelay(upd.Attempts))
	w.finish(ctx, upd, "retry")
}

func (w *Worker) finish(ctx context.Context, d store.Delivery, result string) {
	w.delivered.Inc(result)
	if err := w.store.UpdateDelivery(ctx, d); err != nil {
		w.log.Error("could not record delivery", "delivery", d.ID, "error", err)
	}
}

func (w *Worker) checkpoint(ctx context.Context) error {
	ids, err := w.store.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		if err := w.app.Checkpoint(ctx, ws); err != nil {
			w.log.Error("could not checkpoint ledger", "workspace", ws, "error", err)
		}
	}
	return nil
}

func (w *Worker) retention(ctx context.Context) error {
	if _, err := w.store.DeleteExpiredSessions(ctx); err != nil {
		return err
	}
	ids, err := w.store.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		workspace, err := w.store.GetWorkspace(ctx, ws)
		if err != nil {
			return err
		}
		cutoff := time.Now().Add(-time.Duration(workspace.Settings.RetentionDays) * 24 * time.Hour)
		var total int64
		for {
			n, err := w.store.RedactBefore(ctx, ws, cutoff, 1000)
			if err != nil {
				return err
			}
			total += n
			if n < 1000 || ctx.Err() != nil {
				break
			}
		}
		if total > 0 {
			w.log.Info("redacted action data past retention", "workspace", ws, "actions", total, "cutoff", cutoff)
		}
	}
	return nil
}

func (w *Worker) verify(ctx context.Context) error {
	ids, err := w.store.ListWorkspaceIDs(ctx)
	if err != nil {
		return err
	}
	for _, ws := range ids {
		rep, err := w.app.VerifyWorkspaceLedger(ctx, ws)
		if err != nil {
			w.log.Error("ledger verification errored", "workspace", ws, "error", err)
			continue
		}
		if rep.Break == nil {
			w.log.Info("ledger verified", "workspace", ws, "entries", rep.Entries, "duration_ms", rep.DurationMs)
		}
	}
	return nil
}
