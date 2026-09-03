package domain

import "testing"

func TestApprovalStateMachine(t *testing.T) {
	allowed := []struct {
		from, to VersionState
		role     Role
	}{
		{Draft, Shared, RoleStaff}, {Draft, Shared, RoleOwner}, {Draft, Withdrawn, RoleStaff},
		{Shared, Withdrawn, RoleOwner}, {Shared, RevisionRequested, RoleClient}, {Shared, Approved, RoleClient},
	}
	for _, c := range allowed {
		if err := Transition(c.from, c.to, c.role); err != nil {
			t.Errorf("expected allowed: %v", err)
		}
	}
	denied := []struct {
		from, to VersionState
		role     Role
	}{
		{Shared, Approved, RoleStaff}, {Shared, Approved, RoleOwner}, {Draft, Approved, RoleClient},
		{Draft, Shared, RoleClient}, {Approved, Shared, RoleOwner}, {Approved, Withdrawn, RoleOwner},
		{RevisionRequested, Shared, RoleStaff}, {Withdrawn, Shared, RoleStaff}, {Approved, Draft, RoleOwner},
	}
	for _, c := range denied {
		if err := Transition(c.from, c.to, c.role); err == nil {
			t.Errorf("expected denied: %s → %s as %s", c.from, c.to, c.role)
		}
	}
	if reason, err := NewVersion(Approved); err != nil || !reason {
		t.Errorf("reopen after approval must need a reason: %v %v", reason, err)
	}
	if _, err := NewVersion(Draft); err == nil {
		t.Error("second draft must be rejected")
	}
	if _, err := NewVersion(Shared); err == nil {
		t.Error("new version while shared must be rejected")
	}
	if _, err := NewVersion(""); err != nil {
		t.Error("first version must be allowed")
	}
}

func TestInvoiceTransitions(t *testing.T) {
	if InvoiceTransition(InvoiceDraft, InvoiceSent) != nil || InvoiceTransition(InvoiceSent, InvoicePaid) != nil {
		t.Error("happy path rejected")
	}
	if InvoiceTransition(InvoicePaid, InvoiceSent) == nil || InvoiceTransition(InvoiceDraft, InvoicePaid) == nil || InvoiceTransition(InvoiceCanceled, InvoiceSent) == nil {
		t.Error("illegal invoice move accepted")
	}
}
