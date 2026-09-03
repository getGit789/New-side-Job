// Package domain holds the business rules that must not depend on HTTP or SQL:
// roles, the deliverable approval state machine, and invoice status changes.
// Source of truth: docs/product-contract.md §4 and §5.
package domain

import (
	"errors"
	"fmt"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleStaff  Role = "staff"
	RoleClient Role = "client"
)

func (r Role) IsStaff() bool { return r == RoleOwner || r == RoleStaff }

type VersionState string

const (
	Draft             VersionState = "draft"
	Shared            VersionState = "shared"
	RevisionRequested VersionState = "revision_requested"
	Approved          VersionState = "approved"
	Withdrawn         VersionState = "withdrawn"
)

var ErrTransition = errors.New("transition not allowed")

// Transition reports whether role may move a version from one state to another.
func Transition(from, to VersionState, role Role) error {
	ok := false
	switch {
	case from == Draft && to == Shared, from == Draft && to == Withdrawn, from == Shared && to == Withdrawn:
		ok = role.IsStaff()
	case from == Shared && to == RevisionRequested, from == Shared && to == Approved:
		ok = role == RoleClient
	}
	if !ok {
		return fmt.Errorf("%w: %s → %s as %s", ErrTransition, from, to, role)
	}
	return nil
}

// NewVersion says whether a new draft may be added after the newest version's
// state, and whether a reason is required (reopening an approved deliverable).
// An empty latest state means the deliverable has no versions yet.
func NewVersion(latest VersionState) (needsReason bool, err error) {
	switch latest {
	case "", RevisionRequested, Withdrawn:
		return false, nil
	case Approved:
		return true, nil
	case Draft:
		return false, fmt.Errorf("%w: a draft already exists", ErrTransition)
	default: // shared: the client has not decided yet
		return false, fmt.Errorf("%w: version is shared and awaiting a decision", ErrTransition)
	}
}

type InvoiceStatus string

const (
	InvoiceDraft    InvoiceStatus = "draft"
	InvoiceSent     InvoiceStatus = "sent"
	InvoicePaid     InvoiceStatus = "paid"
	InvoiceCanceled InvoiceStatus = "canceled"
)

func InvoiceTransition(from, to InvoiceStatus) error {
	switch {
	case from == InvoiceDraft && (to == InvoiceSent || to == InvoiceCanceled),
		from == InvoiceSent && (to == InvoicePaid || to == InvoiceCanceled):
		return nil
	}
	return fmt.Errorf("%w: invoice %s → %s", ErrTransition, from, to)
}
