// Package config loads the single canonical configuration: environment variables,
// optionally seeded from a .env file in the working directory.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type SMTP struct {
	Host, User, Pass, From string
	Port                   int
}

type Config struct {
	Env         string
	Addr        string
	BaseURL     *url.URL
	DataDir     string
	DBPath      string
	FilesDir    string
	TrustProxy  bool
	LogLevel    slog.Level
	MaxUploadMB int64
	SMTP        SMTP
}

func (c Config) IsProd() bool { return c.Env == "production" }
func (c Config) IsDemo() bool { return c.Env == "demo" }

// Load reads .env (if present, without overriding real environment variables) then the environment.
func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, err
	}
	var c Config
	var errs []error
	c.Env = env("BRIEFRELAY_ENV", "production")
	if c.Env != "production" && c.Env != "development" && c.Env != "demo" {
		errs = append(errs, fmt.Errorf("BRIEFRELAY_ENV must be production, development or demo (got %q)", c.Env))
	}
	c.Addr = env("BRIEFRELAY_ADDR", "127.0.0.1:8080")
	rawURL := env("BRIEFRELAY_BASE_URL", "http://localhost:8080")
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("BRIEFRELAY_BASE_URL must be an absolute URL (got %q)", rawURL))
	} else {
		u.Path = strings.TrimRight(u.Path, "/")
		c.BaseURL = u
		if c.Env == "production" && u.Scheme != "https" {
			errs = append(errs, errors.New("BRIEFRELAY_BASE_URL must use https in production"))
		}
	}
	c.DataDir, err = filepath.Abs(env("BRIEFRELAY_DATA_DIR", "./data"))
	if err != nil {
		errs = append(errs, err)
	}
	c.DBPath = filepath.Join(c.DataDir, "app.db")
	c.FilesDir = filepath.Join(c.DataDir, "files")
	c.TrustProxy = env("BRIEFRELAY_TRUST_PROXY", "false") == "true"
	if err := c.LogLevel.UnmarshalText([]byte(env("BRIEFRELAY_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("BRIEFRELAY_LOG_LEVEL: %w", err))
	}
	c.MaxUploadMB, err = strconv.ParseInt(env("BRIEFRELAY_MAX_UPLOAD_MB", "50"), 10, 64)
	if err != nil || c.MaxUploadMB < 1 {
		errs = append(errs, errors.New("BRIEFRELAY_MAX_UPLOAD_MB must be a positive integer"))
	}
	c.SMTP.Host = env("BRIEFRELAY_SMTP_HOST", "")
	c.SMTP.User = env("BRIEFRELAY_SMTP_USER", "")
	c.SMTP.Pass = env("BRIEFRELAY_SMTP_PASS", "")
	c.SMTP.From = env("BRIEFRELAY_MAIL_FROM", "BriefRelay <no-reply@localhost>")
	c.SMTP.Port, err = strconv.Atoi(env("BRIEFRELAY_SMTP_PORT", "587"))
	if err != nil {
		errs = append(errs, errors.New("BRIEFRELAY_SMTP_PORT must be an integer"))
	}
	return c, errors.Join(errs...)
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if i := strings.Index(v, " #"); i >= 0 {
			v = v[:i]
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
	return sc.Err()
}
