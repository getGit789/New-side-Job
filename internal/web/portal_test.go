package web

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var inviteRe = regexp.MustCompile(`/invite/([A-Za-z0-9_-]+)`)

// acceptInvite reads the newest invitation email from the job queue and accepts it in a fresh browser.
func acceptInvite(e *env, name string) *browser {
	payload := e.scalar(`SELECT payload FROM jobs WHERE kind = 'mail.send' AND CAST(payload AS TEXT) LIKE '%/invite/%' ORDER BY id DESC LIMIT 1`)
	token := inviteRe.FindStringSubmatch(payload)[1]
	b := e.browser()
	b.want(b.post("/invite/"+token, url.Values{"name": {name}, "password": {"another-long-password"}, "terms": {"1"}}), 303, "accept invite "+name)
	return b
}

func TestClientJourney(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	ownerID := e.scalar(`SELECT id FROM users WHERE email = 'ann@acme.test'`)

	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Hidden"}, "visibility": {"internal"}}), 303, "internal milestone")
	o.want(o.post("/projects/"+pid+"/milestones", url.Values{"title": {"Visible"}, "visibility": {"client"}}), 303, "client milestone")
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}, "required": {"1"}}), 303, "deliverable"))
	did2 := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Brand book"}, "required": {"1"}}), 303, "deliverable 2"))
	o.want(o.upload("/deliverables/"+did+"/versions", nil, "file", "logo.pdf", "%PDF-1.4 v1"), 303, "upload v1")
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ? AND number = 1`, did)
	o.want(o.post("/versions/"+v1+"/comment", url.Values{"body": {"secret-internal-note"}, "visibility": {"internal"}}), 303, "internal comment")
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-INT", "amount": "1", "currency": "USD", "visibility": "internal"}, "", "", ""), 303, "internal invoice")
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-PUB", "amount": "500", "currency": "USD", "visibility": "client"}, "", "", ""), 303, "client invoice")
	pubInv := e.scalar(`SELECT id FROM invoices WHERE number = 'INV-PUB'`)

	// Invite the contact; portal invitations go out through the same queue.
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", url.Values{}), 303, "invite contact")
	c := acceptInvite(e, "Bo")
	if e.scalar(`SELECT COALESCE(user_id, '') FROM client_contacts WHERE id = ?`, ctid) == "" {
		t.Fatal("contact must be linked to the new user")
	}
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", url.Values{}), 422, "cannot invite twice")

	// Client sees their project, not staff pages, not drafts, not internal content.
	if r, body := c.get("/"); r.StatusCode != 200 || !strings.Contains(body, "/portal/projects/"+pid) {
		t.Fatalf("client home: %d", r.StatusCode)
	}
	if r, _ := c.get("/projects/" + pid); r.StatusCode != 403 {
		t.Fatalf("client must not open staff pages, got %d", r.StatusCode)
	}
	if r, _ := c.get("/clients"); r.StatusCode != 403 {
		t.Fatalf("client must not list clients, got %d", r.StatusCode)
	}
	r, body := c.get("/portal/projects/" + pid)
	if r.StatusCode != 200 || strings.Contains(body, "Hidden") || !strings.Contains(body, "Visible") || strings.Contains(body, "INV-INT") {
		t.Fatalf("portal project leaks internal content: %d", r.StatusCode)
	}
	if strings.Contains(body, "/portal/deliverables/"+did) {
		t.Fatal("draft-only deliverable must be invisible to the client")
	}
	if r, _ := c.get("/portal/deliverables/" + did); r.StatusCode != 404 {
		t.Fatalf("draft-only deliverable page must be 404, got %d", r.StatusCode)
	}
	if r, _ := c.get("/portal/versions/" + v1 + "/download"); r.StatusCode != 404 {
		t.Fatalf("draft download must be 404, got %d", r.StatusCode)
	}

	// Share v1: client is notified, can download, comment, and request a revision.
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share v1")
	boID := e.scalar(`SELECT id FROM users WHERE email = 'bo@blue.test'`)
	if e.count(`SELECT count(*) FROM notifications WHERE user_id = ? AND kind = 'version.shared'`, boID) != 1 {
		t.Fatal("client must be notified when a version is shared")
	}
	if e.count(`SELECT count(*) FROM jobs WHERE dedupe_key = ?`, "mail:version.shared:"+v1+":"+boID) != 1 {
		t.Fatal("share email must be queued once with a dedupe key")
	}
	r, body = c.get("/portal/deliverables/" + did)
	if r.StatusCode != 200 || strings.Contains(body, "secret-internal-note") {
		t.Fatalf("internal note leaked to client: %d", r.StatusCode)
	}
	if r, body := c.get("/portal/versions/" + v1 + "/download"); r.StatusCode != 200 || body != "%PDF-1.4 v1" {
		t.Fatalf("client download: %d", r.StatusCode)
	}
	c.want(c.post("/portal/versions/"+v1+"/comment", url.Values{"body": {"Can the blue be darker?"}}), 303, "client comment")
	if e.count(`SELECT count(*) FROM notifications WHERE user_id = ? AND kind = 'comment.client'`, ownerID) != 1 {
		t.Fatal("owner must be notified of client comments")
	}
	c.want(c.post("/portal/versions/"+v1+"/decide", url.Values{"decision": {"revision"}}), 422, "revision needs a note")
	c.want(c.post("/portal/versions/"+v1+"/decide", url.Values{"decision": {"revision"}, "note": {"darker blue please"}}), 303, "request revision")
	c.want(c.post("/portal/versions/"+v1+"/decide", url.Values{"decision": {"approve"}}), 409, "cannot decide twice")
	if e.scalar(`SELECT state FROM deliverable_versions WHERE id = ?`, v1) != "revision_requested" || e.count(`SELECT count(*) FROM decisions WHERE version_id = ?`, v1) != 1 {
		t.Fatal("revision request not recorded")
	}

	// Staff answers with v2; client approves it. v1 keeps its comment and decision.
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v2"}, "", "", ""), 303, "v2 draft")
	v2 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ? AND number = 2`, did)
	if r, body := c.get("/portal/deliverables/" + did); r.StatusCode != 200 || strings.Contains(body, "example.com/v2") {
		t.Fatalf("draft v2 must be hidden from the client: %d", r.StatusCode)
	}
	o.want(o.post("/versions/"+v2+"/share", nil), 303, "share v2")
	c.want(c.post("/portal/versions/"+v2+"/decide", url.Values{"decision": {"approve"}}), 303, "approve v2")
	if e.scalar(`SELECT state FROM deliverable_versions WHERE id = ?`, v2) != "approved" || e.scalar(`SELECT state FROM deliverable_versions WHERE id = ?`, v1) != "revision_requested" {
		t.Fatal("approval must bind to v2 only")
	}
	if e.count(`SELECT count(*) FROM comments WHERE target_id = ?`, v1) != 2 {
		t.Fatal("earlier comments must survive")
	}

	// Intake: draft, submit, clarification round-trip.
	c.want(c.post("/portal/projects/"+pid+"/intake", url.Values{"action": {"submit"}, "goals": {""}}), 422, "submit needs goals")
	c.want(c.post("/portal/projects/"+pid+"/intake", url.Values{"action": {"save"}, "goals": {"draft goals"}}), 303, "save draft")
	c.want(c.post("/portal/projects/"+pid+"/intake", url.Values{"action": {"submit"}, "goals": {"Sell more bread"}}), 303, "submit")
	if e.count(`SELECT count(*) FROM intake_responses WHERE project_id = ? AND status = 'submitted' AND version = 1`, pid) != 1 {
		t.Fatal("intake v1 not submitted")
	}
	o.want(o.post("/projects/"+pid+"/intake/comment", url.Values{"body": {"Which bread?"}}), 303, "clarify")
	if r, body := c.get("/portal/projects/" + pid + "/intake"); r.StatusCode != 200 || !strings.Contains(body, "Which bread?") {
		t.Fatalf("client must see the clarification: %d", r.StatusCode)
	}
	if r, body := o.get("/projects/" + pid); r.StatusCode != 200 || !strings.Contains(body, "Sell more bread") {
		t.Fatalf("staff must see submitted answers: %d", r.StatusCode)
	}

	// Invoice visibility and document rule.
	o.want(o.post("/invoices/"+pubInv+"/status", url.Values{"to": {"sent"}}), 303, "send invoice")
	if e.count(`SELECT count(*) FROM notifications WHERE user_id = ? AND kind = 'invoice.sent'`, boID) != 1 {
		t.Fatal("client must be notified of a sent invoice")
	}
	if r, body := c.get("/portal/projects/" + pid); r.StatusCode != 200 || !strings.Contains(body, "INV-PUB") || strings.Contains(body, "INV-INT") {
		t.Fatalf("invoice visibility: %d", r.StatusCode)
	}

	// Sign-off gate: second required deliverable blocks; owner waives; client signs; project closes.
	c.want(c.post("/portal/projects/"+pid+"/signoff", nil), 409, "sign-off blocked")
	c.want(c.post("/deliverables/"+did2+"/waive", url.Values{"reason": {"x"}}), 403, "client cannot waive")
	o.want(o.post("/deliverables/"+did2+"/waive", url.Values{"reason": {"Dropped from scope"}}), 303, "waive")
	c.want(c.post("/portal/projects/"+pid+"/signoff", nil), 303, "sign off")
	if e.scalar(`SELECT status FROM projects WHERE id = ?`, pid) != "closed" || e.count(`SELECT count(*) FROM signoffs WHERE project_id = ?`, pid) != 1 {
		t.Fatal("sign-off must record and close")
	}
	if snap := e.scalar(`SELECT snapshot FROM signoffs WHERE project_id = ?`, pid); !strings.Contains(snap, "Dropped from scope") || !strings.Contains(snap, `"LatestNumber":2`) {
		t.Fatalf("snapshot incomplete: %s", snap)
	}
	if e.count(`SELECT count(*) FROM notifications WHERE user_id = ? AND kind = 'signoff'`, ownerID) != 1 {
		t.Fatal("owner must be notified of sign-off")
	}
	c.want(c.post("/portal/versions/"+v2+"/comment", url.Values{"body": {"late"}}), 409, "closed project rejects comments")
	c.want(c.post("/portal/projects/"+pid+"/signoff", nil), 409, "cannot sign off a closed project")
	if r, body := c.get("/portal/projects/" + pid); r.StatusCode != 200 || !strings.Contains(body, "Signed off by Bo") {
		t.Fatalf("closed project must still be readable: %d", r.StatusCode)
	}
	if r, _ := c.get("/notifications"); r.StatusCode != 200 {
		t.Fatalf("notifications page: %d", r.StatusCode)
	}

	// Another organization's contact sees nothing of project A.
	cid2 := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Other Co"}}), 303, "client B"))
	o.want(o.post("/clients/"+cid2+"/contacts", url.Values{"name": {"Zed"}, "email": {"zed@other.test"}}), 303, "contact B")
	ctid2 := e.scalar(`SELECT id FROM client_contacts WHERE email = 'zed@other.test'`)
	o.want(o.post("/clients/"+cid2+"/contacts/"+ctid2+"/invite", url.Values{}), 303, "invite B")
	z := acceptInvite(e, "Zed")
	for _, path := range []string{"/portal/projects/" + pid, "/portal/deliverables/" + did, "/portal/versions/" + v1 + "/download", "/portal/projects/" + pid + "/intake", "/portal/invoices/" + pubInv + "/document"} {
		if r, _ := z.get(path); r.StatusCode != 404 {
			t.Fatalf("org B must get 404 for %s, got %d", path, r.StatusCode)
		}
	}
	z.want(z.post("/portal/versions/"+v2+"/comment", url.Values{"body": {"hi"}}), 404, "org B cannot comment")
	if _, body := z.get("/"); strings.Contains(body, pid) {
		t.Fatal("org B home lists project A")
	}

	// Removing a contact ends access at once.
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/remove", nil), 303, "remove contact")
	if r, _ := c.get("/portal/projects/" + pid); r.StatusCode != 303 {
		t.Fatalf("removed contact must be logged out, got %d", r.StatusCode)
	}
}
