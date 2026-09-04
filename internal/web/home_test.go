package web

import (
	"net/url"
	"strings"
	"testing"
)

// The overview points at what needs a hand, for staff and for clients, and never at other people's records.
func TestOverviewPointers(t *testing.T) {
	e := newEnv(t)
	o := setupOwner(e)
	if _, body := o.get("/"); !strings.Contains(body, "First steps") || !strings.Contains(body, "Add a client and a contact") {
		t.Fatal("empty workspace must show the first-steps guide")
	}
	cid := lastSegment(o.want(o.post("/clients", url.Values{"name": {"Blue Bakery"}}), 303, "client"))
	o.want(o.post("/clients/"+cid+"/contacts", url.Values{"name": {"Bo"}, "email": {"bo@blue.test"}}), 303, "contact")
	ctid := e.scalar(`SELECT id FROM client_contacts WHERE email = 'bo@blue.test'`)
	if _, body := o.get("/"); !strings.Contains(body, "Bo at Blue Bakery has no portal access yet") {
		t.Fatal("uninvited contact must be a pointer")
	}
	pid := lastSegment(o.want(o.post("/projects", url.Values{"client_org_id": {cid}, "name": {"Website"}}), 303, "project"))
	did := lastSegment(o.want(o.post("/projects/"+pid+"/deliverables", url.Values{"title": {"Logo"}, "required": {"1"}}), 303, "deliverable"))
	o.want(o.upload("/deliverables/"+did+"/versions", map[string]string{"url": "https://example.com/v1"}, "", "", ""), 303, "draft")
	o.want(o.upload("/projects/"+pid+"/invoices", map[string]string{"number": "INV-1", "amount": "10", "currency": "USD", "visibility": "client", "due_date": "2020-01-01"}, "", "", ""), 303, "invoice")
	iid := e.scalar(`SELECT id FROM invoices WHERE project_id = ?`, pid)
	o.want(o.post("/invoices/"+iid+"/status", url.Values{"to": {"sent"}}), 303, "send")
	_, body := o.get("/")
	for _, want := range []string{"Logo has a draft the client cannot see", "Invoice INV-1 is overdue", "Website: the brief is not started"} {
		if !strings.Contains(body, want) {
			t.Errorf("owner overview lacks %q", want)
		}
	}
	v1 := e.scalar(`SELECT id FROM deliverable_versions WHERE deliverable_id = ?`, did)
	o.want(o.post("/versions/"+v1+"/share", nil), 303, "share")
	o.want(o.post("/clients/"+cid+"/contacts/"+ctid+"/invite", nil), 303, "invite")
	c := acceptInvite(e, "Bo")
	if _, body := o.get("/"); strings.Contains(body, "First steps") {
		t.Fatal("first-steps guide must disappear once every step is done")
	}
	_, body = c.get("/")
	for _, want := range []string{"Logo v1 is waiting for your decision", "Website: tell us what you need", "Invoice INV-1 is open"} {
		if !strings.Contains(body, want) {
			t.Errorf("client overview lacks %q", want)
		}
	}
	c.want(c.post("/portal/versions/"+v1+"/decide", url.Values{"decision": {"approve"}}), 303, "approve")
	if _, body := c.get("/"); !strings.Contains(body, "Website is ready for your sign-off") {
		t.Fatal("client must be pointed at sign-off")
	}
	if _, body := o.get("/"); !strings.Contains(body, "Website is ready for sign-off") {
		t.Fatal("owner must be pointed at sign-off")
	}
	// Staff who are not assigned see none of it.
	o.want(o.post("/team/invite", url.Values{"email": {"sam@acme.test"}}), 303, "staff invite")
	s := acceptInvite(e, "Sam")
	if _, body := s.get("/"); strings.Contains(body, "Website") || strings.Contains(body, "Blue Bakery") {
		t.Fatal("unassigned staff must not see pointers for other projects")
	}
}
