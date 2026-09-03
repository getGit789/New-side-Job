package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestPasswordResetAndChange(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)

	// Forgot: same answer for unknown and known addresses; the known one gets a job with a link.
	anon := e.browser()
	anon.want(anon.post("/password/forgot", url.Values{"email": {"nobody@acme.test"}}), 200, "unknown email")
	if e.count(`SELECT count(*) FROM jobs WHERE kind = 'mail.send'`) != 0 {
		t.Fatal("unknown email must not queue mail")
	}
	anon.want(anon.post("/password/forgot", url.Values{"email": {"ann@acme.test"}}), 200, "known email")
	payload := e.scalar(`SELECT payload FROM jobs WHERE kind = 'mail.send' ORDER BY id DESC LIMIT 1`)
	token := regexp.MustCompile(`/password/reset/([A-Za-z0-9_-]+)`).FindStringSubmatch(payload)[1]
	if r, _ := anon.get("/password/reset/" + token); r.StatusCode != 200 {
		t.Fatalf("reset form: %d", r.StatusCode)
	}
	anon.want(anon.post("/password/reset/"+token, url.Values{"password": {"short"}}), 422, "weak password")
	anon.want(anon.post("/password/reset/"+token, url.Values{"password": {"a-brand-new-password"}}), 303, "reset")
	if r, _ := anon.get("/password/reset/" + token); r.StatusCode != 410 {
		t.Fatalf("reset link must be single use, got %d", r.StatusCode)
	}
	if r, _ := o.get("/projects"); r.StatusCode != 303 {
		t.Fatal("reset must log out every existing session")
	}
	o.want(o.post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"a-long-enough-password"}}), 401, "old password")
	o.want(o.post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"a-brand-new-password"}}), 303, "new password")

	// Change: needs the current password; other sessions end, this one survives.
	other := e.browser()
	other.want(other.post("/login", url.Values{"email": {"ann@acme.test"}, "password": {"a-brand-new-password"}}), 303, "second device")
	o.want(o.post("/account/password", url.Values{"current": {"wrong"}, "password": {"yet-another-password"}}), 401, "re-auth")
	o.want(o.post("/account/password", url.Values{"current": {"a-brand-new-password"}, "password": {"yet-another-password"}}), 200, "change")
	if r, _ := o.get("/account"); r.StatusCode != 200 {
		t.Fatal("changing device must stay logged in")
	}
	if r, _ := other.get("/account"); r.StatusCode != 303 {
		t.Fatal("other device must be logged out")
	}
	if e.count(`SELECT count(*) FROM audit_events WHERE action IN ('auth.reset_requested','auth.reset_completed','user.password_changed')`) != 3 {
		t.Fatal("password events must be audited")
	}
}

func TestExportImportAndDelete(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}, "email": {"hi@blue.test"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}, "required": {"1"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "logo.pdf", "%PDF-1.4 v1"), 303, "v1")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share")
	o.want(o.post("/versions/"+v1+"/comment", url.Values{"body": {"SECRET-INTERNAL"}, "visibility": {"internal"}}), 303, "internal comment")
	o.want(o.post("/versions/"+v1+"/comment", url.Values{"body": {"Visible to client"}, "visibility": {"client"}}), 303, "client comment")
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-INT", "amount": "1", "currency": "USD", "visibility": "internal"}, "", "", ""), 303, "internal invoice")
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	c := acceptInvite(e, "Bo")
	c.want(c.post("/portal/versions/"+v1+"/decide", url.Values{"decision": {"approve"}, "note": {"Great"}}), 303, "approve")
	c.want(c.post("/portal/projects/"+pid+"/signoff", nil), 303, "sign off")

	// AT-11: the archive holds the client-visible record and the file, never internal content.
	r, body := o.get("/projects/" + pid + "/export.zip")
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("export: %d %s", r.StatusCode, r.Header.Get("Content-Type"))
	}
	zr, err := zip.NewReader(bytes.NewReader([]byte(body)), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		names[f.Name] = string(b)
	}
	doc := names["project.json"]
	switch {
	case doc == "":
		t.Fatal("project.json missing")
	case strings.Contains(doc, "SECRET-INTERNAL"), strings.Contains(doc, "INV-INT"):
		t.Fatal("export leaks internal content")
	case !strings.Contains(doc, "Visible to client"), !strings.Contains(doc, `"approved"`), !strings.Contains(doc, "signoff.recorded"), !strings.Contains(doc, "version.shared"):
		t.Fatalf("export is missing client comment, decision, sign-off or audit:\n%s", doc)
	}
	if names["files/versions/Logo-v1-logo.pdf"] != "%PDF-1.4 v1" {
		t.Fatalf("exported file missing or wrong: %v", keys(names))
	}

	// CSV round trip: export, import the same file, nothing duplicated, new rows added.
	r, csvBody := o.get("/clients/export.csv")
	if r.StatusCode != 200 || !strings.Contains(csvBody, "Blue Bakery,hi@blue.test,,,Bo,bo@blue.test,") {
		t.Fatalf("csv export: %d\n%s", r.StatusCode, csvBody)
	}
	csvBody += "New Co,new@co.test,,,Nina,nina@co.test,CEO\n=cmd(),,,,,,\n"
	o.want(o.upload("/clients/import", nil, "file", "clients.csv", csvBody), 303, "import")
	if e.count(`SELECT count(*) FROM client_orgs`) != 3 || e.count(`SELECT count(*) FROM client_contacts`) != 2 {
		t.Fatalf("import counts: orgs=%d contacts=%d", e.count(`SELECT count(*) FROM client_orgs`), e.count(`SELECT count(*) FROM client_contacts`))
	}
	if _, out := o.get("/clients/export.csv"); !strings.Contains(out, "'=cmd()") {
		t.Fatal("formula-like cells must be neutralised on export")
	}
	o.want(o.upload("/clients/import", nil, "file", "bad.csv", "foo,bar\n1,2\n"), 422, "header without name")

	// Deletion: project must be closed (it is, via sign-off); client must be archived first.
	o.want(o.post("/clients/"+cid+"/delete", nil), 422, "delete active client")
	o.want(o.post("/clients/"+cid+"/archive", nil), 303, "archive")
	if r, body := o.get("/clients/" + cid); r.StatusCode != 200 || !strings.Contains(body, "Delete permanently") {
		t.Fatalf("archived client page must offer deletion: %d", r.StatusCode)
	}
	o.want(o.post("/clients/"+cid+"/delete", nil), 303, "delete client")
	if e.count(`SELECT count(*) FROM projects WHERE id = ?`, pid) != 0 || e.count(`SELECT count(*) FROM users WHERE email = 'bo@blue.test'`) != 0 {
		t.Fatal("client deletion must remove projects and portal accounts")
	}
	if e.count(`SELECT count(*) FROM files WHERE deleted_at IS NULL`) != 0 {
		t.Fatal("files of deleted projects must be marked for purge")
	}
	if r, _ := c.get("/"); r.StatusCode != 303 {
		t.Fatal("deleted portal account must be logged out")
	}
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'client_org.deleted'`) != 1 {
		t.Fatal("deletion must be audited")
	}
	// Project delete on an open project is refused; closed works.
	cid2 := e.scalar(`SELECT id FROM client_orgs WHERE name = 'New Co'`)
	pid2 := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid2}, "name": {"Temp"}}), 303, "project 2"))
	o.want(o.post("/projects/"+pid2+"/delete", nil), 422, "delete open project")
	o.want(o.post("/projects/"+pid2+"/close", nil), 303, "close")
	o.want(o.post("/projects/"+pid2+"/delete", nil), 303, "delete closed project")
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
