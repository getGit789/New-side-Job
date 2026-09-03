package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"briefrelay/internal/db"
	"briefrelay/internal/domain"
)

func (s *Server) invoiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /projects/{id}/invoices", s.requireStaff(s.invoiceCreate))
	mux.HandleFunc("POST /invoices/{id}/status", s.requireStaff(s.invoiceStatus))
	mux.HandleFunc("GET /invoices/{id}/document", s.requireStaff(s.invoiceDocument))
}

// parseMoney turns "1234.56" into minor units without floating point.
func parseMoney(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	whole, frac, _ := strings.Cut(s, ".")
	if whole == "" || len(frac) > 2 {
		return 0, errors.New("bad amount")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 {
		return 0, errors.New("bad amount")
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, errors.New("bad amount")
	}
	return w*100 + f, nil
}

func (s *Server) invoiceCreate(w http.ResponseWriter, r *http.Request) {
	p, err := s.loadWritableProject(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	up, err := s.saveUpload(w, r, "document")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := map[string]string{}
	for k := range r.PostForm {
		f[k] = strings.TrimSpace(r.PostForm.Get(k))
	}
	cleanup := func() {
		if up != nil {
			s.store.Delete(up.Info.Key)
		}
	}
	amount, amountErr := parseMoney(f["amount"])
	cur := strings.ToUpper(f["currency"])
	var vErr error
	switch {
	case !within(f["number"], 1, 60):
		vErr = invalid("Invoice number must be 1–60 characters.")
	case amountErr != nil:
		vErr = invalid("Amount must look like 1250.00.")
	case len(cur) != 3:
		vErr = invalid("Currency must be a 3-letter code such as USD.")
	case f["due_date"] != "" && len(f["due_date"]) != 10:
		vErr = invalid("Due date must look like 2026-12-31.")
	case f["visibility"] != "internal" && f["visibility"] != "client":
		vErr = invalid("Visibility must be internal or client.")
	case f["payment_url"] != "":
		if u, err := url.Parse(f["payment_url"]); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(f["payment_url"]) > 2048 {
			vErr = invalid("Payment link must be an absolute http(s) URL.")
		}
	}
	if vErr != nil {
		cleanup()
		s.fail(w, r, vErr)
		return
	}
	id, now, uid := db.NewID(), db.Now(), s.user(r).ID
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var fileID any
		if up != nil {
			if err := insertFile(tx, up, uid); err != nil {
				return err
			}
			fileID = up.ID
		}
		_, err := tx.Exec(`INSERT INTO invoices (id, project_id, number, amount_cents, currency, due_date, payment_url, file_id, visibility, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, p.ID, f["number"], amount, cur, f["due_date"], f["payment_url"], fileID, f["visibility"], uid, now, now)
		return err
	})
	if err != nil {
		cleanup()
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) invoiceStatus(w http.ResponseWriter, r *http.Request) {
	p, err := s.projectOf(r, "invoices", r.PathValue("id"), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	to := domain.InvoiceStatus(f["to"])
	err = s.db.Tx(r.Context(), func(tx *sql.Tx) error {
		var from string
		if err := tx.QueryRow(`SELECT status FROM invoices WHERE id = ?`, r.PathValue("id")).Scan(&from); err != nil {
			return err
		}
		if err := domain.InvoiceTransition(domain.InvoiceStatus(from), to); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE invoices SET status = ?, updated_at = ? WHERE id = ?`, string(to), db.Now(), r.PathValue("id")); err != nil {
			return err
		}
		if err := s.audit(r.Context(), tx, r, "invoice.status_changed", "invoice", r.PathValue("id"), map[string]any{"project_id": p.ID, "from": from, "to": string(to)}); err != nil {
			return err
		}
		var vis, number string
		if err := tx.QueryRow(`SELECT visibility, number FROM invoices WHERE id = ?`, r.PathValue("id")).Scan(&vis, &number); err != nil {
			return err
		}
		if to != domain.InvoiceSent || vis != "client" {
			return nil
		}
		contacts, err := contactsOf(tx, p)
		if err != nil {
			return err
		}
		return s.notify(tx, r, contacts, "invoice.sent", r.PathValue("id"), fmt.Sprintf("Invoice %s for %s", number, p.Name), "/portal/projects/"+p.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/projects/"+p.ID, http.StatusSeeOther)
}

func (s *Server) invoiceDocument(w http.ResponseWriter, r *http.Request) {
	p, err := s.projectOf(r, "invoices", r.PathValue("id"), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var fileID sql.NullString
	if err := s.db.R.QueryRowContext(r.Context(), `SELECT file_id FROM invoices WHERE id = ?`, r.PathValue("id")).Scan(&fileID); err != nil || !fileID.Valid {
		s.fail(w, r, errNotFound)
		return
	}
	s.serveFile(w, r, fileID.String, map[string]any{"project_id": p.ID, "invoice_id": r.PathValue("id")})
}
