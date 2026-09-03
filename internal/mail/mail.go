// Package mail sends plain-text email over SMTP. With no host configured it logs
// instead, which is what development and the public demo want.
package mail

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"briefrelay/internal/config"
)

type Mailer struct {
	cfg config.SMTP
	log *slog.Logger
}

// Job is the payload of a "mail.send" background job.
type Job struct {
	To, Subject, Body string
}

func New(cfg config.SMTP, log *slog.Logger) *Mailer { return &Mailer{cfg: cfg, log: log} }

func (m *Mailer) Configured() bool { return m.cfg.Host != "" }

func (m *Mailer) Send(to, subject, body string) error {
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("mail: bad recipient: %w", err)
	}
	from, err := mail.ParseAddress(m.cfg.From)
	if err != nil {
		return fmt.Errorf("mail: bad BRIEFRELAY_MAIL_FROM: %w", err)
	}
	msg := strings.Join([]string{
		"From: " + m.cfg.From,
		"To: " + to,
		"Subject: " + strings.ReplaceAll(strings.ReplaceAll(subject, "\r", ""), "\n", " "),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")
	if !m.Configured() {
		// Development and demo mode: the body is logged so invitation links can be copied from the log.
		m.log.Info("mail (not sent: SMTP not configured)", "to", to, "subject", subject, "body", body)
		return nil
	}
	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprint(m.cfg.Port))
	var auth smtp.Auth
	if m.cfg.User != "" {
		auth = smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)
	}
	if m.cfg.Port != 465 { // STARTTLS is negotiated automatically when the server offers it
		return smtp.SendMail(addr, auth, from.Address, []string{to}, []byte(msg))
	}
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.cfg.Host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
