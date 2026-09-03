# BriefRelay — Product Contract (Phase 1)

**Status:** draft for freeze. Becomes frozen when Phase 0 passes its gate and the product owner signs section 10.
**Source:** execution plan sections 5, 6.3, 9, 10. Anything not listed here is out of scope for v1.
**Rule of the document:** the server enforces every line below. Hidden buttons are not authorization.

---

## 1. Roles

One installation has exactly one workspace in v1.

| Role | Who | Sees | Can never |
|---|---|---|---|
| Owner | The buyer. Exactly one owner per workspace in v1. | Everything in the workspace | Be removed while they are the only owner |
| Staff | Team members invited by the owner | Only projects they are assigned to | Change workspace settings, team, or exports |
| Client | Contacts of one client organization | Only their organization's projects, and only client-visible content | See internal notes, other organizations, staff-only fields, or the audit log |

A user has one role in the workspace. A client user belongs to exactly one client organization. There is no "staff with all projects" role in v1; the owner assigns staff per project.

---

## 2. Entities and data rules

Every record has: random ID, `created_at`, `updated_at`, and a creator where meaningful. Timestamps are UTC. Money is integer minor units plus an ISO 4217 currency code; no arithmetic is performed on it.

| Entity | Belongs to | Key fields | Removal |
|---|---|---|---|
| Workspace | — | name, logo file, contact details, date format, time zone, default currency | Never in-app |
| User | Workspace (via membership) | email (unique, lower-case), name, password hash | Owner: cannot be removed. Staff: soft-remove membership; audit. Client: soft-remove; audit |
| Membership | Workspace + User | role | With the user |
| Client organization | Workspace | name, contact details, notes (internal) | Soft-delete (`archived_at`) with all projects closed first; audit. Permanent delete is a documented owner action that also deletes contacts, projects, files; audit |
| Client contact | Client organization + User | title | Soft-remove; audit. Their comments and decisions stay |
| Project | Workspace + Client organization | name, status (`active`, `closed`), closed_at, closed_by, brief summary | Close (reversible, audited). Permanent delete only from a closed project by the owner; audit |
| Project member | Project + User (staff) | — | Remove; audit |
| Intake response | Project | version number, status (`draft`, `submitted`), answers, submitted_by, submitted_at | Submitted versions are immutable. Drafts may be discarded by the client |
| Milestone | Project | title, target_date, status (`planned`, `in_progress`, `done`), visibility (`internal`, `client`), latest_update | Delete allowed while `planned`; otherwise mark done. Visibility change is audited |
| Deliverable | Project | title, description, required (bool), sort order | Delete only while every version is `draft`. Otherwise withdraw |
| Deliverable version | Deliverable | number (1..n), kind (`file`, `link`), file_id or url, state (see §4), shared_at/by, decided_at/by, reopen_reason | Never deleted once shared. Draft may be deleted |
| Comment | Deliverable version | author, body, visibility (`internal`, `client`) | Author may delete own comment within 15 minutes; a tombstone stays. Never deleted otherwise |
| Decision | Deliverable version | type (`revision_requested`, `approved`), by, at, note | Immutable |
| Waiver | Deliverable | by (owner), at, reason | Immutable |
| File | Workspace | storage_key, name, size, sha256, media type, uploaded_by | Soft-delete; the blob is removed by a background job after 7 days |
| Invoice | Project | number, amount, currency, due_date, status (`draft`, `sent`, `paid`, `canceled`), payment_url, document file, visibility (`internal`, `client`) | Cancel. Never deleted once `sent` |
| Sign-off | Project | by (client contact), at, checklist snapshot | Immutable |
| Notification | User | kind, target, read_at | Purged after 90 days |
| Audit event | Workspace | actor, action, target, ip, meta | Never deleted in-app; included in export |
| Setting | Workspace | key, value | — |

**Ownership boundary:** every request resolves the target record, walks up to its workspace and (for client-scoped records) its client organization, and checks the caller's role and assignment before any read or write. A record ID from another organization returns `404`, never `403`, so existence is not revealed.

---

## 3. Permission matrix

Columns: **O** owner, **S+** staff assigned to the project, **S−** staff not assigned, **C+** client contact of the project's organization, **C−** any other client. `Y` allowed, `·` denied. Denied means `404` for reads of scoped records and `403` for everything else.

### Workspace and team

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| View / change workspace settings | Y | · | · | · | · |
| Invite staff, remove staff | Y | · | · | · | · |
| View audit log | Y | · | · | · | · |
| Export workspace or project data | Y | · | · | · | · |
| Import clients from CSV | Y | · | · | · | · |
| Change own password / email (re-auth required) | Y | Y | Y | Y | Y |

### Clients and contacts

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Create / edit client organization | Y | · | · | · | · |
| View client organization | Y | Y (only orgs of assigned projects) | · | own org, public fields only | · |
| Invite / remove client contact | Y | Y | · | · | · |
| Archive or permanently delete organization | Y | · | · | · | · |

### Projects

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Create project | Y | Y | Y | · | · |
| View project | Y | Y | · | Y | · |
| Edit project name / summary | Y | Y | · | · | · |
| Assign / unassign staff | Y | · | · | · | · |
| Close project | Y | Y | · | · | · |
| Reopen project (reason required) | Y | · | · | · | · |
| Permanently delete project (closed only) | Y | · | · | · | · |

### Intake

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Save intake draft, submit | · | · | · | Y | · |
| View submitted intake | Y | Y | · | Y | · |
| Request clarification (client-visible comment on intake) | Y | Y | · | · | · |

### Milestones

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Create / edit / delete milestone | Y | Y | · | · | · |
| View `internal` milestone | Y | Y | · | · | · |
| View `client` milestone | Y | Y | · | Y | · |
| Change visibility (audited) | Y | Y | · | · | · |

### Deliverables and versions

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Create deliverable, add draft version | Y | Y | · | · | · |
| Edit draft version | Y | Y | · | · | · |
| Share version (`draft → shared`) | Y | Y | · | · | · |
| Withdraw version | Y | Y | · | · | · |
| View `draft` version | Y | Y | · | · | · |
| View `shared` / later versions | Y | Y | · | Y | · |
| Download version file (re-authorized per request) | Y | Y | · | Y (shared or later only) | · |
| Request revision | · | · | · | Y | · |
| Approve | · | · | · | Y | · |
| Reopen approved deliverable (reason required, audited) | Y | Y | · | · | · |
| Waive a required deliverable (reason required) | Y | · | · | · | · |

### Comments

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Write `internal` comment | Y | Y | · | · | · |
| Read `internal` comment | Y | Y | · | · | · |
| Write `client` comment | Y | Y | · | Y | · |
| Read `client` comment | Y | Y | · | Y | · |
| Delete own comment (15 min, tombstone) | Y | Y | · | Y | · |

### Invoices

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| Create / edit invoice record, attach document | Y | Y | · | · | · |
| Mark sent / paid / canceled (audited) | Y | Y | · | · | · |
| View `client`-visible invoice, download document, open payment link | Y | Y | · | Y | · |
| View `internal` invoice | Y | Y | · | · | · |

### Handoff

| Action | O | S+ | S− | C+ | C− |
|---|---|---|---|---|---|
| View handoff checklist | Y | Y | · | Y | · |
| Sign off | · | · | · | Y | · |
| Export project archive | Y | · | · | · | · |

---

## 4. Approval state machine

State lives on the **deliverable version**. The deliverable's displayed status is the state of its newest version.

```
              share                      approve
  draft ───────────────▶ shared ───────────────────▶ approved
    │                      │  │
    │ withdraw             │  │ request revision
    ▼                      │  ▼
 withdrawn ◀───────────────┘ revision_requested
                                     │
                                     │ staff adds version n+1 (state: draft)
                                     ▼
                                   draft  (on the new version; the old one stays revision_requested)

  approved ──reopen (reason)──▶ new version n+1 in draft; the approved version stays approved
```

| From | To | Who | Side effects |
|---|---|---|---|
| draft | shared | O, S+ | version content becomes immutable; client notification + email; audit |
| draft | withdrawn | O, S+ | audit |
| shared | withdrawn | O, S+ | client notification; audit |
| shared | revision_requested | C+ | Decision row; staff notification + email; audit |
| shared | approved | C+ | Decision row; staff notification + email; audit |
| revision_requested | (new draft version) | O, S+ | previous comments and decisions untouched |
| approved | (new draft version) | O, S+ | reason required; audit `deliverable.reopened` |

Everything else is rejected with `409 Conflict` and the current state.

**Invariants**

1. A version in any state other than `draft` cannot change file, link, title, or kind.
2. A Decision references exactly one version and is never updated or deleted.
3. A deliverable has at most one version in `draft` at a time.
4. Version numbers are gap-free and assigned inside the write transaction.
5. Approving version *n* does not change the state of any other version.
6. Reopening never deletes or edits the approved version or its decision.

---

## 5. Project lifecycle and sign-off

```
 active ──close──▶ closed ──reopen (owner, reason)──▶ active
```

- **Sign-off is available** only when every deliverable with `required = true` has its newest version `approved`, or carries a Waiver from the owner.
- Sign-off stores a snapshot: list of required deliverables, their approved version numbers, waivers, contact who signed, timestamp, IP. The snapshot is immutable.
- Sign-off automatically closes the project.
- A **closed** project is read-only for every role. Downloads still work. Comments, uploads, decisions, and edits return `409`.
- Reopen requires a reason, is owner-only, is audited, and does not remove the sign-off record. A second sign-off creates a second record.
- Sign-off is an operational record. The product never claims legal e-signature status, in UI or docs.

---

## 6. Global invariants (tested as a matrix, release blockers)

1. A client of organization A can never list, read, search, download, comment on, or infer the existence of organization B's records. Cross-org IDs return `404`.
2. Staff not assigned to a project get `404` for that project's scoped records.
3. Internal comments, internal milestones, internal invoices, and client-organization notes never appear in client pages, client notifications, client emails, search results for clients, or project archives exported for clients.
4. Every file download re-checks authorization at request time; there are no long-lived public file URLs. Expiring download links, if used, are single-purpose and bound to the user.
5. Changing any ID in a request URL or body never crosses the workspace, organization, or assignment boundary.
6. Every table in §3 is executed as an automated test: for each row, each column, one positive or negative assertion. The suite fails the release if any cell disagrees.

---

## 7. Audit and notification catalog

Audit events (always recorded, owner-visible, exported):

`auth.login`, `auth.login_failed`, `auth.logout`, `auth.password_changed`, `auth.email_changed`, `invitation.sent`, `invitation.accepted`, `invitation.revoked`, `member.removed`, `client_org.created`, `client_org.archived`, `client_org.deleted`, `project.created`, `project.closed`, `project.reopened`, `project.deleted`, `milestone.visibility_changed`, `version.shared`, `version.withdrawn`, `decision.revision_requested`, `decision.approved`, `deliverable.reopened`, `deliverable.waived`, `invoice.status_changed`, `file.downloaded`, `file.deleted`, `signoff.recorded`, `export.created`, `settings.changed`, `setup.completed`.

Notifications (in-app always; email per user preference, default on):

| Event | Notifies |
|---|---|
| Version shared | all contacts of the org |
| Revision requested / approved | assigned staff + owner |
| Client comment | assigned staff + owner |
| Staff client-visible comment | all contacts |
| Intake submitted | assigned staff + owner |
| Invoice marked sent | all contacts |
| Sign-off recorded | assigned staff + owner |
| Invitation | the invitee (email only) |

Every email job carries a dedupe key `mail:<event>:<record id>:<recipient id>`. The same event never emails the same person twice.

---

## 8. Validation rules

| Field | Rule |
|---|---|
| Email | RFC 5322 shape, max 254, stored lower-case |
| Password | min 12 chars, max 200, checked against the top 10k breached list shipped with the product |
| Names and titles | 1–200 chars, trimmed |
| Free text (comments, notes, updates) | max 10,000 chars, stored as plain text, rendered escaped |
| URL (links, payment) | absolute `http` or `https` only, max 2,048 |
| File upload | size ≤ `BRIEFRELAY_MAX_UPLOAD_MB`; extension allow-list from settings; detected media type must not be executable or HTML; stored under a random name outside the web root |
| Money | integer minor units ≥ 0; currency is a 3-letter code from the shipped list |
| Dates | ISO 8601 date; displayed in workspace time zone and format |
| Lists | page size 25, max 100; every list endpoint is paginated |
| Invitation / reset tokens | 256-bit random, hashed at rest, single use, expire in 7 days (invite) / 1 hour (reset), revocable |

---

## 9. Acceptance tests (Phase 3 and 4 exit)

Written as automated end-to-end tests. Numbering is stable and referenced from stories.

**AT-1 Install.** Fresh data directory → `/setup` → owner created → `/setup` returns 404 → `/healthz` is 200.
**AT-2 Invite client.** Owner creates org + contact → invitation email job enqueued with dedupe key → link accepted once → second use returns 410 → audit shows `invitation.accepted`.
**AT-3 Intake.** Contact saves draft twice → submits → version 1 immutable → staff sees it → contact from org B gets 404.
**AT-4 Milestone visibility.** Staff creates internal milestone → client list excludes it → visibility set to client → client sees it → audit shows the change.
**AT-5 Share and approve (signature flow).** Staff uploads v1 → shares → client notification + email job exist → client requests revision with comment → staff cannot edit v1 → staff adds v2 → shares → client approves v2 → decision references v2 → v1 still `revision_requested` with its comment intact.
**AT-6 No silent replacement.** After AT-5, staff attempts to change v2's file → 409. Staff reopens with reason → v3 draft created → v2 still approved → audit shows `deliverable.reopened` with reason.
**AT-7 Sign-off gate.** Project with two required deliverables, one approved → sign-off unavailable → owner waives the other with reason → sign-off available → client signs → snapshot lists v2 and the waiver → project closed → comment attempt returns 409.
**AT-8 Invoice.** Staff records invoice with document and payment link → client downloads document and sees link → staff marks paid → audit shows status change → internal invoice invisible to client.
**AT-9 Isolation matrix.** Generated from §3: every cell asserted for every role using seeded orgs A and B.
**AT-10 Download authorization.** Client B requests client A's file by ID → 404. Removed contact requests a file they previously downloaded → 404.
**AT-11 Export and close.** Owner exports project archive → archive contains client-visible content, decisions, sign-off, audit; contains no internal comments.
**AT-12 Recovery.** Kill the process during an upload → restart → no orphan file record; temp file cleaned by the scheduled job.

---

## 10. Sign-off of this contract

| Role | Name | Date | Decision |
|---|---|---|---|
| Product owner | | | |
| Senior engineer | | | |

Change control after freeze: any change to §3, §4, §5, or §6 needs a new row here and a new migration note.
