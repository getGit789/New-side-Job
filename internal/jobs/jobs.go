// Package jobs is a SQLite-backed background queue with retries, idempotency keys,
// and a scheduler for recurring work. It runs inside the web process so buyers
// need only one thing to keep alive.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"briefrelay/internal/db"
)

type Handler func(ctx context.Context, payload []byte) error

type Queue struct {
	db        *db.DB
	log       *slog.Logger
	handlers  map[string]Handler
	every     []recurring
	lastTick  atomic.Int64 // unix seconds; health reads this
	Poll      time.Duration
	StaleLock time.Duration
}

type recurring struct {
	kind     string
	interval time.Duration
}

func New(d *db.DB, log *slog.Logger) *Queue {
	return &Queue{db: d, log: log, handlers: map[string]Handler{}, Poll: time.Second, StaleLock: 10 * time.Minute}
}

func (q *Queue) Handle(kind string, h Handler) { q.handlers[kind] = h }

// Every enqueues kind once per interval, keyed by the interval bucket so restarts never duplicate it.
func (q *Queue) Every(kind string, interval time.Duration) {
	q.every = append(q.every, recurring{kind, interval})
}

// Options for Enqueue. DedupeKey makes a second enqueue with the same key a no-op forever.
type Options struct {
	DedupeKey   string
	RunAt       time.Time
	MaxAttempts int
}

// Enqueue inserts a job inside the caller's transaction, so a job is only visible if the business write commits.
func Enqueue(ctx context.Context, tx *sql.Tx, kind string, payload any, o Options) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if o.RunAt.IsZero() {
		o.RunAt = time.Now()
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	var key any
	if o.DedupeKey != "" {
		key = o.DedupeKey
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs (kind, payload, dedupe_key, run_at, max_attempts, created_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(dedupe_key) DO NOTHING`,
		kind, body, key, db.Time(o.RunAt), o.MaxAttempts, db.Now())
	return err
}

// Healthy reports whether the worker loop ticked recently.
func (q *Queue) Healthy() bool {
	return time.Since(time.Unix(q.lastTick.Load(), 0)) < 3*q.Poll+5*time.Second
}

// Run blocks until ctx is done, processing jobs one at a time.
func (q *Queue) Run(ctx context.Context) {
	t := time.NewTicker(q.Poll)
	defer t.Stop()
	for {
		q.lastTick.Store(time.Now().Unix())
		q.schedule(ctx)
		for {
			worked, err := q.RunOne(ctx)
			if err != nil && ctx.Err() == nil {
				q.log.Error("job loop", "err", err)
			}
			if !worked || ctx.Err() != nil {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (q *Queue) schedule(ctx context.Context) {
	if len(q.every) == 0 {
		return
	}
	err := q.db.Tx(ctx, func(tx *sql.Tx) error {
		for _, r := range q.every {
			bucket := time.Now().Truncate(r.interval)
			if err := Enqueue(ctx, tx, r.kind, nil, Options{DedupeKey: fmt.Sprintf("every:%s:%d", r.kind, bucket.Unix()), RunAt: bucket}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && ctx.Err() == nil {
		q.log.Error("schedule", "err", err)
	}
}

// RunOne claims and runs a single ready job. worked is false when the queue is empty.
func (q *Queue) RunOne(ctx context.Context) (worked bool, err error) {
	var id int64
	var kind string
	var payload []byte
	var attempts, maxAttempts int
	now := time.Now()
	err = q.db.Tx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT id, kind, payload, attempts, max_attempts FROM jobs WHERE done_at IS NULL AND run_at <= ? AND (locked_at IS NULL OR locked_at < ?) ORDER BY run_at, id LIMIT 1`,
			db.Time(now), db.Time(now.Add(-q.StaleLock)))
		if err := row.Scan(&id, &kind, &payload, &attempts, &maxAttempts); err != nil {
			return err
		}
		attempts++
		_, err := tx.ExecContext(ctx, `UPDATE jobs SET locked_at = ?, attempts = ? WHERE id = ?`, db.Time(now), attempts, id)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	h, ok := q.handlers[kind]
	runErr := fmt.Errorf("no handler for job kind %q", kind)
	if ok {
		runErr = q.safeRun(ctx, h, payload)
	}
	return true, q.db.Tx(ctx, func(tx *sql.Tx) error {
		if runErr == nil {
			_, err := tx.ExecContext(ctx, `UPDATE jobs SET done_at = ?, locked_at = NULL, last_error = NULL WHERE id = ?`, db.Now(), id)
			return err
		}
		q.log.Warn("job failed", "id", id, "kind", kind, "attempt", attempts, "err", runErr)
		if attempts >= maxAttempts {
			_, err := tx.ExecContext(ctx, `UPDATE jobs SET done_at = ?, locked_at = NULL, last_error = ? WHERE id = ?`, db.Now(), "dead: "+runErr.Error(), id)
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE jobs SET run_at = ?, locked_at = NULL, last_error = ? WHERE id = ?`, db.Time(time.Now().Add(Backoff(attempts))), runErr.Error(), id)
		return err
	})
}

func (q *Queue) safeRun(ctx context.Context, h Handler, payload []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, payload)
}

// Backoff: 30s, 2m, 8m, 32m ... capped at 6h.
func Backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt && d < 6*time.Hour; i++ {
		d *= 4
	}
	return min(d, 6*time.Hour)
}

// Stats is what the health endpoint exposes: counts only, never payloads.
type Stats struct {
	Pending int `json:"pending"`
	Dead    int `json:"dead"`
}

func (q *Queue) Stats(ctx context.Context) (s Stats, err error) {
	err = q.db.R.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM jobs WHERE done_at IS NULL),
		(SELECT count(*) FROM jobs WHERE last_error LIKE 'dead: %')`).Scan(&s.Pending, &s.Dead)
	return s, err
}

// Cleanup deletes finished jobs older than 30 days. Registered as a recurring job by main.
func (q *Queue) Cleanup(ctx context.Context, _ []byte) error {
	_, err := q.db.W.ExecContext(ctx, `DELETE FROM jobs WHERE done_at IS NOT NULL AND done_at < ?`, db.Time(time.Now().Add(-30*24*time.Hour)))
	return err
}
