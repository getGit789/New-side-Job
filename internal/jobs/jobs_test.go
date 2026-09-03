package jobs

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"briefrelay/internal/db"
)

func newQueue(t *testing.T) *Queue {
	d, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return New(d, slog.Default())
}

func TestDedupeRetryAndDead(t *testing.T) {
	ctx := context.Background()
	q := newQueue(t)
	q.StaleLock = 0
	calls := 0
	q.Handle("flaky", func(ctx context.Context, payload []byte) error {
		calls++
		if string(payload) != `{"to":"a@b.c"}` {
			t.Fatalf("payload %s", payload)
		}
		return errors.New("smtp down")
	})
	for range 2 { // same dedupe key twice: only one row
		err := q.db.Tx(ctx, func(tx *sql.Tx) error {
			return Enqueue(ctx, tx, "flaky", map[string]string{"to": "a@b.c"}, Options{DedupeKey: "mail:1", MaxAttempts: 2})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if s, _ := q.Stats(ctx); s.Pending != 1 {
		t.Fatalf("dedupe failed: %+v", s)
	}
	if worked, err := q.RunOne(ctx); !worked || err != nil {
		t.Fatalf("attempt 1: worked=%v err=%v", worked, err)
	}
	if worked, _ := q.RunOne(ctx); worked {
		t.Fatal("retry must wait for backoff")
	}
	q.db.W.Exec(`UPDATE jobs SET run_at = ?`, db.Time(time.Now().Add(-time.Second)))
	if worked, _ := q.RunOne(ctx); !worked {
		t.Fatal("attempt 2 should run")
	}
	s, _ := q.Stats(ctx)
	if calls != 2 || s.Pending != 0 || s.Dead != 1 {
		t.Fatalf("calls=%d stats=%+v", calls, s)
	}
}

func TestUnknownKindAndPanicDoNotCrash(t *testing.T) {
	ctx := context.Background()
	q := newQueue(t)
	q.Handle("boom", func(context.Context, []byte) error { panic("x") })
	q.db.Tx(ctx, func(tx *sql.Tx) error {
		Enqueue(ctx, tx, "boom", nil, Options{MaxAttempts: 1})
		return Enqueue(ctx, tx, "nope", nil, Options{MaxAttempts: 1})
	})
	for range 2 {
		if _, err := q.RunOne(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if s, _ := q.Stats(ctx); s.Dead != 2 {
		t.Fatalf("stats=%+v", s)
	}
}

func TestBackoffCap(t *testing.T) {
	if Backoff(1) != 30*time.Second || Backoff(2) != 2*time.Minute || Backoff(50) != 6*time.Hour {
		t.Fatal("backoff schedule changed")
	}
}
