# Release checklist

Every item must pass before a build goes to customers (plan §10 and §18). Automated gates run in CI;
manual ones are done once per release candidate and ticked here with the date and tester.

## Automated gates (`make check` and `make perf`)

- [ ] `make lint`: gofmt clean, `go vet` clean, `govulncheck` reports no vulnerability in the shipped build.
  Known and accepted: GO-2026-5932 flags `golang.org/x/crypto/openpgp` as unmaintained. BriefRelay imports only
  `argon2` from that module; the OpenPGP package is never compiled in. No upstream fix exists.
- [ ] `make test`: all packages pass with the race detector.
  - Permission matrix: every route × owner / assigned staff / unassigned staff / client A / client B / anonymous (`internal/web/matrix_test.go`). A route not in the matrix fails the build.
  - State machine and version rules (`internal/domain`).
  - Owner, staff and client journeys end to end (`internal/web/*_test.go`).
  - Backup and restore drill, including path-traversal rejection (`internal/backup`).
  - Upgrade from the first release schema with data, and migrations append-only (`internal/db`).
  - Template accessibility: every control labelled, `lang` set, `<main>` landmark (`internal/web/a11y_test.go`).
- [ ] `make perf`: all pages inside budget at the seeded scale; numbers recorded in `docs/performance.md`.
- [ ] `./dist/briefrelay check` passes on the release binary.

## Manual tests

| Test | Steps | Pass |
|---|---|---|
| Fresh install from docs only | New machine or container. Follow README only. Setup, invite a client, share a version, approve, sign off. | |
| Upgrade with rollback practice | Install previous release with data. Back up. Install candidate. Verify. Roll back to the backup. Verify. | |
| Owner, staff, client smoke | Current and previous major of Chrome, Firefox, Safari, Edge. | |
| Keyboard only, client actions | Tab through invitation accept, open version, comment, approve, request revision, download, submit brief, sign off. No mouse. Focus visible on every control. | |
| Screen reader smoke | NVDA or VoiceOver: invitation, comment, approval, sign-off. Every control announced with its label; status read as text. | |
| Mobile web, client actions | Phone-width viewport: review, comment, approve, download. | |
| Email rendering and retry | Configure real SMTP. Invite; check the mail. Stop SMTP; invite; see retries in `/healthz` job stats; start SMTP; job completes once. | |
| Upload and download limits | Upload at `BRIEFRELAY_MAX_UPLOAD_MB`; one byte over is refused with a clear message. HTML and SVG uploads refused. Downloads arrive as attachments. | |
| Recovery | Kill the server mid-upload. Restart. No orphan file row; temp file removed by the hourly job (or `files.cleanup`). | |
| Demo reset and anti-abuse | Phase 6. | |
| Clean-package audit | `tar tzf` the release archive: binary, README, `.env.example`, docs only. No `.env`, no database, no personal data. `SHA256SUMS` matches. | |

## Release blockers (any one stops the release)

- Cross-client data exposure, or any authentication / authorization bypass.
- Data loss, a failed restore, or a migration that cannot be rolled back.
- Approval history that can be altered without an audit row.
- Install failure on the documented environment, or a broken core workflow in the shipped archive.
- High or critical vulnerability in the shipped configuration.
- Missing or wrong licence attribution (`dist/dependencies.txt`).
- A page over its performance budget without an approved explanation.

## Still open before the marketplace release

- Independent security review (plan §6.3): not yet commissioned.
- Project archive export and permanent deletion paths (Phase 6).
- Password reset and re-authentication for owner security changes (Epic 2): not built. Owners change
  passwords through a database reset today; this must be closed before launch.
