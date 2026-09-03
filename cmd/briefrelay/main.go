// briefrelay is a single binary: web server, background worker, and scheduler.
//
//	briefrelay serve    run the application (default)
//	briefrelay migrate  apply pending database migrations and exit
//	briefrelay check    preflight: verify the environment without starting
//	briefrelay backup FILE.tar.gz   snapshot the database and uploaded files (safe while running)
//	briefrelay restore FILE.tar.gz  unpack a backup into an empty data directory
//	briefrelay version
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"briefrelay/internal/auth"
	"briefrelay/internal/backup"
	"briefrelay/internal/config"
	"briefrelay/internal/db"
	"briefrelay/internal/jobs"
	"briefrelay/internal/mail"
	"briefrelay/internal/storage"
	"briefrelay/internal/web"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	if cmd == "version" {
		fmt.Println(web.Version)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:\n ", err)
		os.Exit(2)
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	var run func(config.Config, *slog.Logger) error
	switch cmd {
	case "serve":
		run = serve
	case "migrate":
		run = migrate
	case "check":
		run = check
	case "backup", "restore":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "usage: briefrelay %s FILE.tar.gz\n", cmd)
			os.Exit(2)
		}
		run = func(cfg config.Config, log *slog.Logger) error { return backupCmd(cfg, cmd, os.Args[2]) }
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (serve | migrate | check | backup | restore | version)\n", cmd)
		os.Exit(2)
	}
	if err := run(cfg, log); err != nil {
		log.Error("fatal", "cmd", cmd, "err", err)
		os.Exit(1)
	}
}

func migrate(cfg config.Config, log *slog.Logger) error {
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer d.Close()
	applied, err := d.Migrate(context.Background())
	log.Info("migrate", "applied", applied)
	return err
}

func check(cfg config.Config, log *slog.Logger) error {
	var failed bool
	report := func(name string, err error, warn bool) {
		switch {
		case err == nil:
			fmt.Printf("  ok    %s\n", name)
		case warn:
			fmt.Printf("  warn  %s: %v\n", name, err)
		default:
			failed = true
			fmt.Printf("  FAIL  %s: %v\n", name, err)
		}
	}
	fmt.Printf("BriefRelay %s preflight (env=%s)\n", web.Version, cfg.Env)
	st, err := storage.NewLocal(cfg.FilesDir)
	if err == nil {
		err = st.Writable()
	}
	report("data directory writable: "+cfg.DataDir, err, false)
	d, err := db.Open(cfg.DBPath)
	report("database opens: "+cfg.DBPath, err, false)
	if err == nil {
		defer d.Close()
		applied, err := d.Migrate(context.Background())
		report(fmt.Sprintf("database migrated (%d applied now)", len(applied)), err, false)
	}
	var mailErr error
	if cfg.SMTP.Host == "" {
		mailErr = errors.New("BRIEFRELAY_SMTP_HOST is empty; mail will be logged, not sent")
	}
	report("outbound mail configured", mailErr, true)
	var proxyErr error
	if cfg.IsProd() && !cfg.TrustProxy {
		proxyErr = errors.New("BRIEFRELAY_TRUST_PROXY=false; rate limits use the proxy's IP if one is in front")
	}
	report("reverse proxy trust", proxyErr, true)
	if failed {
		return errors.New("preflight failed")
	}
	fmt.Println("Preflight passed.")
	return nil
}

func backupCmd(cfg config.Config, cmd, file string) error {
	if cmd == "restore" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := backup.Restore(f, cfg.DataDir); err != nil {
			return err
		}
		fmt.Printf("restored %s into %s\n", file, cfg.DataDir)
		return nil
	}
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer d.Close()
	f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := backup.Write(context.Background(), d, cfg.FilesDir, f); err != nil {
		f.Close()
		os.Remove(file)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("backup written to %s\n", file)
	return nil
}

func serve(cfg config.Config, log *slog.Logger) error {
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer d.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if applied, err := d.Migrate(ctx); err != nil {
		return err
	} else if len(applied) > 0 {
		log.Info("migrations applied", "versions", applied)
	}
	st, err := storage.NewLocal(cfg.FilesDir)
	if err != nil {
		return err
	}
	m := mail.New(cfg.SMTP, log)

	q := jobs.New(d, log)
	q.Handle("jobs.cleanup", q.Cleanup)
	q.Every("jobs.cleanup", 24*time.Hour)
	q.Handle("sessions.cleanup", func(ctx context.Context, _ []byte) error { return auth.DeleteExpiredSessions(ctx, d) })
	q.Every("sessions.cleanup", time.Hour)
	q.Handle("mail.send", func(ctx context.Context, payload []byte) error {
		var j mail.Job
		if err := json.Unmarshal(payload, &j); err != nil {
			return err
		}
		return m.Send(j.To, j.Subject, j.Body)
	})
	q.Handle("files.cleanup", func(ctx context.Context, _ []byte) error { return cleanupFiles(ctx, d, st) })
	q.Every("files.cleanup", time.Hour)

	s, err := web.New(cfg, d, q, m, st, log)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr: cfg.Addr, Handler: s.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 2 * time.Minute,
		MaxHeaderBytes: 64 << 10,
	}

	workerDone := make(chan struct{})
	go func() { defer close(workerDone); q.Run(ctx) }()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()
	log.Info("listening", "addr", cfg.Addr, "base_url", cfg.BaseURL.String(), "env", cfg.Env, "version", web.Version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
		<-workerDone
		return err
	}
	<-workerDone
	log.Info("stopped")
	return nil
}

// cleanupFiles removes blobs of files soft-deleted more than 7 days ago and abandoned upload temp files.
func cleanupFiles(ctx context.Context, d *db.DB, st *storage.Local) error {
	if err := st.CleanTemp(24 * time.Hour); err != nil {
		return err
	}
	rows, err := d.R.QueryContext(ctx, `SELECT id, storage_key FROM files WHERE deleted_at IS NOT NULL AND deleted_at < ? LIMIT 500`, db.Time(time.Now().Add(-7*24*time.Hour)))
	if err != nil {
		return err
	}
	type row struct{ id, key string }
	var doomed []row
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.id, &x.key); err == nil {
			doomed = append(doomed, x)
		}
	}
	rows.Close()
	for _, x := range doomed {
		if err := st.Delete(x.key); err != nil {
			return err
		}
		if err := d.Tx(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(`DELETE FROM files WHERE id = ?`, x.id); return err }); err != nil {
			return err
		}
	}
	return nil
}
