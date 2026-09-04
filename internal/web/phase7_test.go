package web

import (
	"net/url"
	"strings"
	"testing"
)

// Phase 7: the plan §5.4 and contract items that were still open after Phase 6.

func TestWorkspaceSettings(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}}), 303, "deliverable"))

	base := map[string]string{"name": "Acme Studio", "contact": "Acme, 1 Main St", "time_zone": "Europe/Berlin", "date_format": "02.01.2006", "currency": "eur", "allowed_ext": "pdf, PNG"}
	o.want(o.upload("/settings", base, "logo", "logo.png", "\x89PNG\r\n\x1a\n0000IHDR"), 303, "save settings")
	bad := map[string]string{}
	for k, v := range base {
		bad[k] = v
	}
	bad["time_zone"] = "Mars/Olympus"
	o.want(o.upload("/settings", bad, "", "", ""), 422, "unknown time zone")
	bad["time_zone"], bad["date_format"] = "UTC", "yyyy"
	o.want(o.upload("/settings", bad, "", "", ""), 422, "unknown date format")
	o.want(o.upload("/settings", base, "logo", "logo.pdf", "%PDF-1.4 not an image"), 422, "logo must be an image")

	// Name, contact, logo and default currency reach the pages; dates render in the workspace zone and format.
	r, body := o.get("/projects/" + pid)
	if r.StatusCode != 200 {
		t.Fatalf("project page: %d", r.StatusCode)
	}
	for _, want := range []string{"Acme Studio", `src="/logo"`, `value="EUR"`} {
		if !strings.Contains(body, want) {
			t.Errorf("project page lacks %q", want)
		}
	}
	if _, login := e.browser().get("/login"); !strings.Contains(login, "Acme, 1 Main St") || !strings.Contains(login, "Acme Studio") {
		t.Error("login page must show the workspace name and contact details")
	}
	if created := e.scalar(`SELECT created_at FROM projects WHERE id = ?`, pid); !strings.Contains(body, created[8:10]+"."+created[5:7]+"."+created[:4]) {
		t.Errorf("dates must use the workspace format (created %s):\n%s", created, body)
	}
	if r, _ := e.browser().get("/logo"); r.StatusCode != 200 || !strings.HasPrefix(r.Header.Get("Content-Type"), "image/png") {
		t.Fatalf("logo: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	// The allow-list applies to every upload; the deny-list still wins.
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "draft.docx", "PK\x03\x04"), 422, "docx not on the allow-list")
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "v1.pdf", "%PDF-1.4 v1"), 303, "pdf allowed")
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'settings.changed'`) != 1 {
		t.Fatal("settings change must be audited")
	}
}

func TestAccountEmailAndNotificationPreference(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	o.want(o.post("/account/email", url.Values{"current": {"wrong"}, "email": {"new@acme.test"}}), 401, "re-auth")
	o.want(o.post("/account/email", url.Values{"current": {"a-long-enough-password"}, "email": {"not-an-email"}}), 422, "bad email")
	o.want(o.post("/account/email", url.Values{"current": {"a-long-enough-password"}, "email": {"New@Acme.test"}}), 200, "change email")
	if e.scalar(`SELECT email FROM users WHERE name = 'Ann'`) != "new@acme.test" || e.count(`SELECT count(*) FROM audit_events WHERE action = 'auth.email_changed'`) != 1 {
		t.Fatal("email change must be stored lower-case and audited")
	}

	// A client who turns email off still gets in-app notifications but no mail job.
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}}), 303, "deliverable"))
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	c := acceptInvite(e, "Bo")
	c.want(c.post("/account/notifications", url.Values{}), 200, "email off")
	mailsBefore := e.count(`SELECT count(*) FROM jobs WHERE kind = 'mail.send'`)
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v1"}, "", "", ""), 303, "v1")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share")
	if e.count(`SELECT count(*) FROM notifications WHERE kind = 'version.shared'`) != 1 {
		t.Fatal("in-app notification must always be written")
	}
	if e.count(`SELECT count(*) FROM jobs WHERE kind = 'mail.send'`) != mailsBefore {
		t.Fatal("no mail may be queued when email notifications are off")
	}
	c.want(c.post("/account/notifications", url.Values{"email_notifications": {"1"}}), 200, "email on")
	o.want(o.post("/versions/"+v1+"/comment", url.Values{"body": {"hello"}, "visibility": {"client"}}), 303, "client-visible comment")
	if e.count(`SELECT count(*) FROM jobs WHERE kind = 'mail.send'`) != mailsBefore+1 {
		t.Fatal("mail must be queued again once email notifications are on")
	}
}

func TestCommentDeleteWindowAndTombstone(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v1"}, "", "", ""), 303, "v1")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share")
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	c := acceptInvite(e, "Bo")

	c.want(c.post("/portal/versions/"+v1+"/comment", url.Values{"body": {"client says hi"}}), 303, "client comment")
	clientComment := e.scalar(`SELECT id FROM comments WHERE body = 'client says hi'`)
	o.want(o.post("/versions/"+v1+"/comment", url.Values{"body": {"staff says hi"}, "visibility": {"client"}}), 303, "staff comment")
	staffComment := e.scalar(`SELECT id FROM comments WHERE body = 'staff says hi'`)

	if _, body := c.get("/portal/deliverables/" + did); !strings.Contains(body, "/portal/comments/"+clientComment+"/delete") || strings.Contains(body, "/portal/comments/"+staffComment+"/delete") {
		t.Fatal("client must see a delete button only on their own fresh comment")
	}
	o.want(o.post("/comments/"+clientComment+"/delete", nil), 422, "owner cannot delete a client's comment")
	c.want(c.post("/portal/comments/"+staffComment+"/delete", nil), 422, "client cannot delete staff comment")
	c.want(c.post("/portal/comments/"+clientComment+"/delete", nil), 303, "client deletes own comment")
	if _, body := o.get("/deliverables/" + did); strings.Contains(body, "client says hi") || !strings.Contains(body, "(deleted)") {
		t.Fatal("deleted comment must leave a tombstone, not its text")
	}
	// Older than 15 minutes: refused.
	e.db.W.Exec(`UPDATE comments SET created_at = '2020-01-01T00:00:00Z' WHERE id = ?`, staffComment)
	o.want(o.post("/comments/"+staffComment+"/delete", nil), 422, "too late to delete")
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'comment.deleted'`) != 1 {
		t.Fatal("comment deletion must be audited")
	}
}

func TestInvoiceEditAndDeliverableDelete(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))

	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-1", "amount": "10", "currency": "USD", "visibility": "client"}, "", "", ""), 303, "invoice")
	iid := e.scalar(`SELECT id FROM invoices WHERE project_id = ?`, pid)
	o.want(o.upload("/invoices/"+iid, map[string]string{"number": "INV-1A", "amount": "12.50", "currency": "eur", "visibility": "client", "due_date": "2026-12-31"}, "document", "inv.pdf", "%PDF-1.4 doc"), 303, "edit draft")
	if e.scalar(`SELECT number || ' ' || amount_cents || ' ' || currency || ' ' || due_date FROM invoices WHERE id = ?`, iid) != "INV-1A 1250 EUR 2026-12-31" {
		t.Fatal("invoice edit not stored")
	}
	if r, body := o.get("/invoices/" + iid + "/document"); r.StatusCode != 200 || body != "%PDF-1.4 doc" {
		t.Fatalf("attached document: %d %q", r.StatusCode, body)
	}
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"sent"}}), 303, "send")
	o.want(o.upload("/invoices/"+iid, map[string]string{"number": "INV-1B", "amount": "12.50", "currency": "EUR", "visibility": "client"}, "", "", ""), 303, "edit sent")
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"paid"}}), 303, "pay")
	o.want(o.upload("/invoices/"+iid, map[string]string{"number": "INV-1C", "amount": "1", "currency": "EUR", "visibility": "client"}, "", "", ""), 409, "paid invoice is frozen")
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'invoice.updated'`) != 2 {
		t.Fatal("invoice edits must be audited")
	}

	// Deliverable delete: fine with drafts only, refused once anything was shared.
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "v1.pdf", "%PDF-1.4 v1"), 303, "draft")
	if _, body := o.get("/deliverables/" + did); !strings.Contains(body, "/deliverables/"+did+"/delete") {
		t.Fatal("delete button missing while only drafts exist")
	}
	o.want(o.post("/deliverables/"+did+"/delete", nil), 303, "delete draft-only deliverable")
	if e.count(`SELECT count(*) FROM deliverables`) != 0 || e.count(`SELECT count(*) FROM files WHERE deleted_at IS NULL AND name = 'v1.pdf'`) != 0 {
		t.Fatal("deliverable and its draft file must be gone")
	}
	did = lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}}), 303, "deliverable 2"))
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v1"}, "", "", ""), 303, "v1")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share")
	o.want(o.post("/deliverables/"+did+"/delete", nil), 409, "shared deliverable cannot be deleted")
}

func TestInviteTermsAndCommonPassword(t *testing.T) {
	e := newEnv(t)
	b := e.browser()
	b.want(b.post("/setup", url.Values{"workspace": {"Acme"}, "name": {"Ann"}, "email": {"ann@acme.test"}, "password": {"unbelievable"}}), 422, "common password at setup")
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	payload := e.scalar(`SELECT payload FROM jobs WHERE kind = 'mail.send' ORDER BY id DESC LIMIT 1`)
	token := inviteRe.FindStringSubmatch(payload)[1]
	c := e.browser()
	if _, body := c.get("/invite/" + token); !strings.Contains(body, `name="terms"`) {
		t.Fatal("client invitation must show the terms notice")
	}
	c.want(c.post("/invite/"+token, url.Values{"name": {"Bo"}, "password": {"another-long-password"}}), 422, "terms not accepted")
	c.want(c.post("/invite/"+token, url.Values{"name": {"Bo"}, "password": {"unbelievable"}, "terms": {"1"}}), 422, "common password")
	c.want(c.post("/invite/"+token, url.Values{"name": {"Bo"}, "password": {"another-long-password"}, "terms": {"1"}}), 303, "accepted")
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'invitation.accepted' AND json_extract(meta, '$.terms_accepted') = 1`) != 1 {
		t.Fatal("terms acceptance must be recorded in the audit event")
	}
	// Staff invitations carry no client notice.
	o.want(o.post("/team/invite", url.Values{"email": {"sam@acme.test"}}), 303, "staff invite")
	payload = e.scalar(`SELECT payload FROM jobs WHERE kind = 'mail.send' ORDER BY id DESC LIMIT 1`)
	if _, body := c.get("/invite/" + inviteRe.FindStringSubmatch(payload)[1]); strings.Contains(body, `name="terms"`) {
		t.Fatal("staff invitation must not show the client notice")
	}
	o.want(o.post("/account/password", url.Values{"current": {"a-long-enough-password"}, "password": {"unbelievable"}}), 422, "common password on change")
}

func TestSearchIsScoped(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client A"))
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Bakery site"}}), 303, "project A"))
	o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Bakery logo"}}), 303, "deliverable A")
	cid2 := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Secret Bakery"}}), 303, "client B"))
	pid2 := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid2}, "name": {"Secret bakery site"}}), 303, "project B"))
	o.want(o.post("/projects/"+pid2+"/deliverables", url.Values{"title": {"Secret bakery logo"}}), 303, "deliverable B")
	o.want(o.post("/team/invite", url.Values{"email": {"sam@acme.test"}}), 303, "invite staff")
	s := acceptInvite(e, "Sam")
	o.want(o.post("/projects/"+pid+"/members", url.Values{"user_id": {e.scalar(`SELECT id FROM users WHERE email = 'sam@acme.test'`)}}), 303, "assign")

	_, body := o.get("/search?q=bakery")
	for _, want := range []string{"Blue Bakery", "Secret Bakery", "Bakery site", "Secret bakery site", "Bakery logo", "Secret bakery logo"} {
		if !strings.Contains(body, want) {
			t.Errorf("owner search lacks %q", want)
		}
	}
	_, body = s.get("/search?q=bakery")
	if strings.Contains(body, "Secret") || !strings.Contains(body, "Bakery logo") || !strings.Contains(body, "Blue Bakery") {
		t.Fatalf("staff search must show only assigned records:\n%s", body)
	}
	if _, body := s.get("/search?q=%25"); strings.Contains(body, "Secret") {
		t.Fatal("wildcard characters must be literal")
	}
}
