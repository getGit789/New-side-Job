# BriefRelay — Business, Product, and Execution Plan

**Status:** Ready for product validation and engineering planning  
**Research snapshot:** September 3, 2026  
**Commercial target:** 100 paid licenses at a $29 list price  
**Working title:** BriefRelay. Treat this as provisional until trademark, marketplace-name, and domain checks are complete.  
**Design boundary:** Visual design, colors, typography, illustration, and aesthetic direction are intentionally excluded.

---

## 1. Executive decision

Build **a self-hosted client approval and project handoff portal for freelancers and small agencies**, sold as a one-time source-code license.

The product gives a service business one private place to:

- onboard a client;
- show project status and milestones;
- share files, links, and deliverable versions;
- collect comments, revision requests, approvals, and final sign-off;
- show invoice status and an external payment link; and
- archive and export the completed engagement.

The buyer hosts the product on their own infrastructure and keeps using the installed version without a creator subscription. The primary acquisition channel is **CodeCanyon**, where buyers already search for installable code products and can use a live demo before buying. The seller's existing Stripe account remains useful for a separate direct-sales channel, but the first 100-customer plan does not depend on building an audience from scratch.

### Product promise

> Replace scattered client emails, file links, status requests, and ambiguous approvals with one self-hosted client space for a one-time $29 purchase.

### Why this product

The opportunity sits between two unsatisfying choices:

1. Broad CRM and project-management systems are capable but expensive and overloaded.
2. Lightweight hosted client portals are simple, but often charge every month and keep the customer's data on someone else's service.

BriefRelay is deliberately narrower: **client delivery, feedback, approval, and handoff**. It is not another full CRM.

---

## 2. Evidence behind the decision

### 2.1 Demand exists

- Upwork estimates that roughly **20 million U.S. skilled knowledge workers performed freelance work in 2024**, producing more than **$1.5 trillion in earnings**. This is a large, durable population of people who repeatedly manage client work. [Upwork Future Workforce Index](https://www.upwork.com/research/future-workforce-index-2025)
- Upwork also reports that **49% of full-time workers rely on freelancers to address critical gaps**, while **48% of CEOs planned to increase freelance hiring**. More freelance work creates more client-delivery workflows. [Upwork in-demand skills research](https://www.upwork.com/research/in-demand-skills-2025)
- CodeCanyon's project-management category shows proven willingness to buy self-hosted business software: Perfex CRM is listed at $89 with roughly 25,900 sales, RISE at $89 with roughly 8,000 sales, and WORKSUITE at $60 with roughly 4,800 sales. [CodeCanyon project-management tools](https://codecanyon.net/category/php-scripts/project-management-tools)
- A narrower $29 CRM product has already passed 195 sales, showing that the exact entry price can convert in this marketplace. [CodeCanyon CRM results](https://codecanyon.net/category/php-scripts/project-management-tools?term=crm)

These figures prove category demand, not guaranteed demand for this exact product. That is why the plan contains a validation gate before full implementation.

### 2.2 There is a useful gap

- A CodeCanyon/ThemeForest search for “client portal” currently returns only 18 results across all categories; 13 are classified by the marketplace as high-sales items. The result set is much thinner than generic admin-dashboard or CRM categories. [Envato client-portal search](https://themeforest.net/search/client%20portal)
- A current focused “Client Manager — CRM, Billing & Client Portal” product is listed at $69. [CodeCanyon client-dashboard search](https://codecanyon.net/search/client%20dashboard)
- A presentation-only client-portal template on Framer is listed at $99. [Framer ClientHub listing](https://www.framer.com/marketplace/templates/clienthub/)
- Hosted lightweight competitors charge monthly. HandoffHQ advertises paid plans beginning around $12 per month when billed annually, while ClientDock advertises a £19-per-month Pro plan. [HandoffHQ](https://handoffhq.app/) and [ClientDock](https://www.clientdock.pro/)

The gap is therefore not “no competitors.” It is **a focused, working, self-hosted portal at an impulse-level one-time price**.

### 2.3 Why not the obvious alternatives

| Alternative | Evidence | Decision |
|---|---|---|
| Generic Notion CRM template | Notion currently shows about 1,860 CRM templates, while capable paid examples can be as low as $7. [Notion CRM marketplace](https://www.notion.com/en-gb/templates/category/crm) | Too saturated and too easy to imitate; weak moat at $29. |
| Generic portfolio or agency website template | Framer's marketplace already contains many polished free and paid templates, commonly around $29–$129. [Framer portfolio templates](https://www.framer.com/templates/categories/portfolio/) | High supply and purchase is driven heavily by visual differentiation, which is outside this plan. |
| Multipurpose admin dashboard | ThemeForest lists more than 2,700 admin dashboards. [ThemeForest admin templates](https://themeforest.net/category/site-templates/admin-templates) | Proven demand, but severe competition and feature-count arms races. |
| Full CRM | Very strong sales, but mature leaders already contain hundreds of features. | Too large for a simple first product and too expensive to support well at $29. |
| Focused client approval and handoff portal | Recurring SaaS competitors, higher-priced templates, and relatively thin marketplace supply. | **Selected.** Narrow enough to execute, useful enough to justify $29, and differentiated by ownership. |

---

## 3. Commercial thesis

### Target buyer

The primary buyer is a technically comfortable freelancer or owner of a 2–15 person service business who handles 3–30 active clients and can install a self-hosted product or ask their developer to do it.

Priority segments:

1. web and software studios;
2. brand and design studios;
3. marketing and content agencies;
4. video and production freelancers;
5. consultants who deliver files, reports, or staged engagements.

Do not target large enterprises, consumers, or buyers seeking a complete accounting, sales, or HR system.

### Jobs to be done

The buyer hires BriefRelay to:

- stop answering “where is the latest file?”;
- stop rebuilding a client portal for every project;
- make project status visible without another meeting;
- create an auditable approval trail;
- make final handoff feel controlled and professional;
- keep client data on infrastructure they control; and
- avoid another monthly software bill.

### Positioning statement

For freelancers and small agencies that deliver project work, BriefRelay is a self-hosted client approval and handoff portal that centralizes status, files, feedback, and sign-off. Unlike broad CRMs and monthly client-portal services, it focuses only on the delivery workflow and is purchased once.

### Defensible advantages

- **Narrow workflow:** onboarding-to-sign-off rather than an all-purpose CRM.
- **No creator subscription:** ownership of the installed version is the central offer.
- **Self-hosted data:** a strong reason to choose the product beyond price.
- **Fast adoption:** sample data, setup checks, guided first project, and concise documentation.
- **Useful live demo:** buyers can experience both owner and client roles before purchase.
- **Low operating dependency:** the product remains useful without a creator-run API or cloud service.

---

## 4. The $29 offer

### Regular license offer

**List price:** $29, one-time.

Included:

- source code for one licensed end product;
- owner, staff, and client roles;
- all version 1 features in this plan;
- demo data and a setup wizard;
- English installation, administration, user, update, and troubleshooting documentation;
- future released updates available to existing buyers under marketplace rules; and
- six months of marketplace-defined item support if support is enabled.

Not included:

- hosting;
- installation as a service;
- custom development;
- data migration from another product;
- legal, tax, or compliance certification;
- guaranteed future features; or
- indefinite one-to-one support.

“Own it forever” must be stated accurately: the buyer receives an ongoing license for the permitted end product and can keep using the version they downloaded. Envato says its licenses continue for the life of the end product, provided the license terms are followed. [Envato license duration](https://help.market.envato.com/hc/en-us/articles/115005597546-Do-licenses-have-an-expiry-date-Will-I-ever-have-to-pay-any-fees-like-renewals-or-royalties)

### Extended license

Offer an extended license at a marketplace-appropriate one-time price for buyers who will charge their own end users to access a product containing BriefRelay. Final pricing should be set after the marketplace category and its fixed buyer fee are confirmed. Envato distinguishes regular and extended licenses based primarily on whether end users pay to access the resulting product. [Envato license guide](https://help.market.envato.com/hc/en-us/articles/115005593363-Do-I-need-a-Regular-License-or-an-Extended-License)

### Price anchor

The sales page may truthfully compare categories of alternatives:

- $29 once for BriefRelay;
- roughly $12–£19 every month for lightweight hosted alternatives;
- $69 for a current CodeCanyon client-manager product;
- $60–$89 for established broad CRM/project-management products; and
- $99 for a current presentation-only Framer client-portal template.

Do not claim a dollar amount “saved” unless the exact competitor plan and comparison period are clearly disclosed.

### Financial reality

At 100 sales, customer spend is **$2,900 before tax and handling fees**. That is not the same as seller earnings.

Envato currently charges authors 50% of the item-price component. CodeCanyon's fixed buyer fee is currently $2–$5 for common code categories; for a $29 listing, that implies estimated author earnings of approximately **$12–$13.50 per regular sale**, or **$1,200–$1,350 for 100 marketplace sales**, before tax and withholding. [Envato author fee](https://help.author.envato.com/hc/en-us/articles/360000472943-Introduction-to-Earnings) and [CodeCanyon fixed buyer fees](https://help.author.envato.com/hc/en-us/articles/360000473203-Fixed-Buyer-Fees-on-Envato-Market)

Therefore track two separate goals:

- **Market proof goal:** 100 paid customers at $29.
- **Cash goal:** marketplace net proceeds, reported separately from gross customer spend.

If the intended goal is $2,900 net rather than 100 customers, the marketplace target is closer to 215–242 regular-license sales under the current fee structure. A separate direct Stripe channel can retain more revenue per sale, but it will require independent traffic and merchant-of-record/tax handling.

---

## 5. Version 1 product scope

### 5.1 Roles

| Role | Purpose | Access boundary |
|---|---|---|
| Owner | Configures the workspace and controls all records | Full access, including team, settings, exports, and audit history |
| Staff | Delivers assigned client work | Only permitted clients, projects, files, comments, and actions |
| Client | Reviews their own engagement | Only their organization's projects and explicitly shared content |

Authorization must be enforced on the server for every object and action. Hidden navigation is not authorization.

### 5.2 Core entities

- Workspace
- User
- Workspace membership and role
- Client organization
- Client contact
- Project
- Project member
- Intake response
- Milestone
- Deliverable
- Deliverable version
- Comment
- Approval or revision request
- File or external link
- Invoice record and external payment URL
- Notification
- Activity and audit event
- Workspace setting

Every business record must have a stable identifier, creator, timestamps, current status, and ownership boundary. Records that can be removed must define whether removal is reversible, auditable, or permanent.

### 5.3 Required workflows

#### A. Install and configure

1. Buyer uploads or deploys the product.
2. A requirements check confirms the environment is ready.
3. The setup flow creates the owner account and initial workspace.
4. The buyer configures outbound email and file storage.
5. The setup locks itself after successful completion.
6. A health check confirms database, storage, email, and scheduled-job readiness.

**Acceptance target:** a technically comfortable buyer can reach the sample dashboard in 10 minutes using the documentation and a supported environment.

#### B. Create and invite a client

1. Owner creates a client organization and at least one contact.
2. Owner creates a project from a blank state or included workflow preset.
3. Owner sends a single-use, expiring invitation.
4. Client sets credentials and accepts the applicable terms notice.
5. The owner sees the accepted invitation in the activity history.

#### C. Collect an intake brief

1. Owner selects the included project-intake fields.
2. Client saves a draft and submits when ready.
3. Staff can request clarification.
4. The final submission is retained as a versioned project record.

The initial release needs only a fixed, useful intake structure with optional fields. A general form builder is out of scope.

#### D. Report project progress

1. Staff creates milestones with target dates and status.
2. Client sees the approved public status and latest update.
3. Internal notes remain inaccessible to clients.
4. Any visibility change is auditable.

#### E. Review and approve a deliverable

1. Staff uploads a deliverable version or attaches an external link.
2. Client receives an in-app and email notification.
3. Client comments, requests revision, or approves.
4. A revision creates a new version without destroying earlier versions.
5. Approval records who approved, what version, and when.
6. Staff cannot silently replace an approved version.

This is the product's signature workflow and must receive the strongest automated test coverage.

#### F. Show invoice status

1. Staff records invoice number, amount, currency, due date, status, and optional external payment link.
2. Client can view or download the supplied invoice document and open the external payment URL.
3. Staff marks the invoice paid or canceled.

Version 1 does not calculate tax, reconcile payments, process cards, or perform accounting.

#### G. Final handoff and sign-off

1. Staff marks required deliverables complete.
2. Client sees a final handoff checklist and approves completion.
3. The system records immutable sign-off metadata.
4. Owner exports a project archive and closes the project.
5. Closed projects are read-only until deliberately reopened by an authorized user.

Sign-off is an operational record, not a promised legally compliant electronic signature.

### 5.4 Supporting capabilities

- Search and filtering across clients, projects, and deliverables.
- Paginated lists; no unbounded record loading.
- In-app notification center and configurable email notifications.
- File download permissions and expiring download links where supported.
- Workspace name, logo, contact details, date format, time zone, and default currency settings.
- Import of clients from a documented comma-separated file.
- Export of client and project records in durable formats.
- Audit history for security-sensitive and approval-related actions.
- Clear empty, loading, success, validation, permission-denied, and failure states.
- Translation-ready text organization, while version 1 ships in English only.

### 5.5 Explicitly out of scope for version 1

- multi-tenant SaaS operation;
- creator-hosted storage, email, analytics, or licensing API;
- native card or bank payment processing;
- bookkeeping, tax calculation, or financial reporting;
- lead pipeline, sales CRM, proposals, or contracts;
- time tracking, payroll, attendance, or HR;
- live chat, voice, or video calls;
- a general workflow or form builder;
- artificial-intelligence features;
- native mobile applications;
- marketplace or vendor management;
- public developer API;
- automation builder;
- custom code editor;
- simultaneous real-time document editing; and
- third-party integrations beyond outbound email, file storage, and ordinary external links.

Reject requests that turn version 1 into a broad CRM. The narrow scope is the commercial strategy.

---

## 6. Non-functional product requirements

The senior engineer owns language, framework, database, and deployment choices. This plan defines outcomes and gates, not those implementation decisions.

### 6.1 Performance budgets

All performance claims must be reproduced on a documented reference environment and with a seeded data set of at least 500 clients, 5,000 projects, 25,000 deliverable versions, and 100,000 activity events.

- Public demo pages: current Core Web Vitals “good” thresholds at the 75th percentile.
- Authenticated read operations: p95 server response at or below 300 ms, excluding network transit and third-party services.
- Authenticated write operations: p95 server response at or below 500 ms, excluding file transfer and third-party services.
- Search: first page of results at or below 500 ms at the seeded scale.
- Ordinary page transition: meaningful content usable within 1.5 seconds on the reference environment and a typical broadband connection.
- No list view may fetch an unbounded collection.
- Background work such as email delivery, archive generation, and media inspection must not block the user's request.
- Performance regression fails the release gate if p95 latency worsens by more than 15% without an approved explanation.

### 6.2 Reliability and recovery

- Database changes must be versioned, repeatable, and backward-safe for the documented upgrade path.
- Every update must include pre-update checks, backup instructions, migration steps, rollback guidance, and a changelog.
- A failed background job must retry safely without duplicating business actions.
- Email and notification actions must be idempotent where duplication would confuse the client.
- File metadata and stored file state must be reconciled after interrupted uploads or deletions.
- A backup-and-restore drill must pass before release.
- A health endpoint must report application, database, storage, scheduled-job, and mail readiness without exposing secrets.

### 6.3 Security and privacy

- Apply the current OWASP application-security guidance at release time.
- Deny access by default; test every role/object/action combination.
- Use established password hashing, secure session handling, request-forgery protection, output encoding, input validation, and rate limits.
- Invitation, reset, download, and approval links must be random, expiring, single-purpose, and revocable where appropriate.
- Require re-authentication for owner email, password, or security-setting changes.
- Never ship default production credentials or secrets.
- Keep uploaded files outside directly executable public paths; validate size, extension, and detected media type; randomize stored names.
- Do not record passwords, tokens, file contents, or payment details in logs.
- Record authentication, permission, invitation, download, deletion, export, approval, and owner-setting events in the audit history.
- Document data export, client removal, account removal, backups, retention, and permanent deletion.
- Generate a dependency inventory and machine-readable bill of materials for every release.
- Block a release for unresolved critical or high-severity vulnerabilities that affect the shipped configuration.
- Commission an independent security-focused review before the marketplace release.

### 6.4 Accessibility and compatibility

- All core workflows must be usable by keyboard.
- Inputs need programmatic labels, useful error messages, and predictable focus behavior.
- Status must not rely only on visual styling.
- Automated accessibility checks plus manual keyboard and screen-reader smoke tests are release requirements.
- Support the current and previous major versions of the agreed mainstream browsers at the release date.
- Mobile web must support client review, comment, approval, and file download. Full owner administration on very small screens is not a launch requirement.

### 6.5 Maintainability

- One canonical configuration path and one documented installation path.
- Clear separation between domain rules, storage, email, background work, and presentation.
- Centralized authorization policies and validation rules.
- No unexplained copied code or abandoned dependencies.
- Automated formatting, static analysis, tests, dependency checks, and package creation on every release candidate.
- Reproducible release packages with checksums and version identifiers.
- Meaningful error messages for buyers, with detailed server logs for diagnosis.

---

## 7. Logical infrastructure plan

This is platform-neutral. The senior engineer will select the implementation technologies through an architecture decision record.

### Required runtime responsibilities

| Responsibility | Requirement |
|---|---|
| Web application | Serves owner, staff, and client workflows; remains stateless wherever practical |
| Primary data store | Transactional source of truth for accounts, permissions, projects, approvals, and audit data |
| File storage | Private by default; supports authorization before download and documented local or external storage configuration |
| Background worker | Handles email, archive generation, cleanup, and retryable jobs |
| Scheduler | Runs cleanup, reminders, and maintenance without requiring user traffic |
| Cache | Optional optimization; correctness must not depend on cached state |
| Email transport | Buyer-configured; failures are visible and retryable |
| Reverse proxy and TLS | Production traffic is encrypted; proxy behavior and trusted headers are documented |
| Observability | Structured logs, health checks, job-failure visibility, and basic operational counters |
| Backup process | Covers database, files, and configuration secrets with a documented restore procedure |

### Environments

- **Development:** local work and automated checks.
- **Test:** disposable environment for automated integration and end-to-end tests.
- **Staging:** release-candidate environment matching the documented production shape.
- **Public demo:** synthetic data only, outbound abuse paths disabled, automatic reset on a fixed schedule.
- **Release build:** clean environment used only to create and verify the customer package.

### Technology-selection gate owned by the senior engineer

The chosen stack must be scored and documented against:

1. measured runtime performance on the target hosting class;
2. ease of installation for CodeCanyon buyers;
3. current security support and dependency health;
4. quality of authentication, authorization, migration, queue, email, and test tooling;
5. ability to run without a creator-operated cloud service;
6. predictable resource use;
7. maintainability by a small team;
8. packaging and upgrade simplicity; and
9. marketplace category fit and buyer expectations.

“Fastest language” is not accepted as a conclusion without an end-to-end benchmark of the actual product workload. The decision record must include a small representative benchmark and the installation/support tradeoff.

---

## 8. Delivery plan

### Team assumptions

- One senior engineer owns architecture and implementation.
- One product/project owner owns scope, acceptance, marketplace submission, and beta coordination.
- Security and release reviews can be performed by a second qualified reviewer.
- The visual design is supplied separately and is not on the critical path once its states and responsive behavior are complete.

### Schedule: six to seven working weeks

| Phase | Duration | Outputs | Exit gate |
|---|---:|---|---|
| 0. Validation | 3 days | Competitor review mining, 10 buyer conversations, feature ranking, price test | At least 5 interviewees report the problem monthly or more; at least 3 agree to test the beta |
| 1. Product contract | 2 days | Final requirements, permission matrix, data rules, acceptance tests, architecture decision brief | No unresolved scope or ownership ambiguity |
| 2. Engineering foundation | 5 days | Installation path, authentication, roles, data model, migration system, jobs, storage, mail, test pipeline | Fresh install and basic security tests pass |
| 3. Owner and staff workflow | 7 days | Clients, projects, intake, milestones, deliverables, invoice records, activity | Owner can run a complete seeded project |
| 4. Client workflow | 5 days | Invitation, client access, review, comments, revisions, approval, handoff | End-to-end client journey passes |
| 5. Hardening | 5 days | Permission audit, performance tuning, accessibility, backup/restore, upgrade test, dependency review | All release quality gates pass |
| 6. Packaging and marketplace | 5 days | Demo, documentation, sample data, release archive, listing copy, preview assets, support process | A new tester installs without live help; submission package passes audit |
| 7. Review buffer | 3–5 days | Marketplace corrections and launch-blocker fixes | Product accepted or all review feedback addressed |

### Epic backlog

#### Epic 1 — Installation and ownership

- Environment requirements check.
- Guided initial setup.
- Owner creation.
- Configuration validation.
- Locked post-install state.
- Health and diagnostics page.
- Backup, restore, and update documentation.

#### Epic 2 — Identity and authorization

- Login, logout, reset, and invitation flows.
- Owner, staff, and client roles.
- Assignment-based staff access.
- Client-organization isolation.
- Session and security-event history.
- Rate limiting and recovery controls.

#### Epic 3 — Clients and projects

- Client organizations and contacts.
- Projects and membership.
- Fixed intake questionnaire.
- Milestones and public status updates.
- Internal versus client-visible content.
- Archive and reopen controls.

#### Epic 4 — Deliverables and approvals

- File/link deliverables.
- Immutable version history.
- Threaded comments scoped to a version.
- Revision request.
- Approval and final sign-off.
- Notification and audit events.

#### Epic 5 — Financial reference

- Invoice metadata.
- Invoice document.
- External payment link.
- Manual status changes with audit history.
- Currency display without accounting logic.

#### Epic 6 — Operations and release

- Background jobs and scheduler.
- Logging and health checks.
- Seeded demo and periodic reset.
- Export and deletion paths.
- Documentation and sample data.
- Reproducible release package.

### Critical path

1. Final permission matrix.
2. Data model and installation path.
3. Authentication and organization isolation.
4. Deliverable versioning and approval state machine.
5. End-to-end owner/client journey.
6. Security, performance, install, upgrade, and restore gates.
7. Demo, documentation, and marketplace submission.

Do not build invoice extras, integrations, or additional presets while any critical-path item is incomplete.

---

## 9. Product rules and acceptance criteria

### Approval state machine

Allowed deliverable states:

`draft → shared → revision requested → shared → approved`

Alternative terminal path:

`draft/shared → withdrawn`

Rules:

- Only staff can share or withdraw.
- Only an authorized client contact can request revision or approve.
- A shared version cannot be edited in place.
- A new revision does not remove prior comments or decisions.
- Approval binds to one exact version.
- Reopening an approved deliverable creates an audit event and requires a reason.
- Final project sign-off is unavailable until required deliverables are approved or explicitly waived by the owner with a recorded reason.

### Permission invariants

- A client from organization A can never enumerate, view, search, download, comment on, or infer the existence of organization B's records.
- Staff access is limited to assigned clients/projects unless the owner grants a broader documented role.
- Internal notes never appear in client exports, notifications, search results, or API responses.
- A file download is re-authorized at request time.
- Changing a record identifier in a request never crosses an ownership boundary.

### Definition of done for every story

- Acceptance criteria pass.
- Permission behavior is tested.
- Validation and failure states are handled.
- Audit and notification behavior is correct where applicable.
- Automated tests cover the business rule.
- No new high or critical security finding.
- Documentation is updated if buyer behavior changes.
- Performance budget is not regressed.
- Product owner accepts the result in staging.

---

## 10. Quality and release strategy

### Automated test layers

- Unit tests for state transitions, validation, permissions, and calculations.
- Integration tests for database rules, mail, jobs, storage, imports, and exports.
- End-to-end tests for install, invite, login, project creation, deliverable revision, approval, handoff, and recovery.
- Permission tests using a full role/object/action matrix.
- Migration tests from the previous supported release to the candidate release.
- Package test from the exact archive that customers will download.
- Performance test against the seeded benchmark data.
- Security checks for dependencies, secrets, unsafe files, common web attacks, and authorization bypass.

### Manual release tests

- Fresh installation using only customer documentation.
- Upgrade from the prior release with real backup and rollback practice.
- Owner, staff, and client smoke test in supported browsers.
- Keyboard-only completion of all client-critical actions.
- Screen-reader smoke test for invitation, comment, approval, and sign-off.
- Email rendering and failure/retry behavior.
- File upload/download behavior at allowed limits.
- Demo reset and anti-abuse controls.
- Clean-package audit for secrets, personal data, development files, and unlicensed assets.

### Release blockers

- Cross-client data exposure.
- Authentication or authorization bypass.
- Data loss, broken restore, or irreversible failed migration.
- Approval history that can be silently altered.
- Installation failure on the documented supported environment.
- Critical workflow failure in the shipped archive.
- High or critical applicable security vulnerability.
- Missing or inaccurate licensing attribution.
- Material performance budget failure.

---

## 11. Demo and customer package

### Public demo

Provide two obvious test paths:

1. **Agency owner demo:** create or inspect clients, milestones, deliverables, invoices, and activity.
2. **Client demo:** review a project, comment, request a revision, approve, and sign off.

Demo safeguards:

- synthetic data only;
- fixed, published reset interval;
- no real outbound email;
- strict upload type and size limits, or uploads disabled;
- rate limits and abuse monitoring;
- no ability to change demo owner security settings;
- visible sample credentials that are automatically restored; and
- no secrets or reusable production configuration.

Envato describes the live preview as an important part of the buying experience and requires any supplied preview to function correctly. [Envato presentation requirements](https://help.author.envato.com/hc/en-us/articles/360000424863-Item-Presentation-Requirements)

### Download package

- Versioned application source package.
- Example configuration with no secrets.
- Environment requirements and preflight checker.
- Database creation and migration assets.
- Sample-data option.
- English HTML or PDF installation guide.
- Owner and client user guide.
- Update, backup, restore, and rollback guide.
- Troubleshooting and support-scope guide.
- Changelog.
- Third-party licenses and attributions.
- Dependency inventory and bill of materials.
- Release checksum.

CodeCanyon requires basic English documentation covering installation, customization, use, and asset credits. [Code item requirements](https://help.author.envato.com/hc/en-us/articles/360000471583-Code-Item-Preparation-Technical-Requirements)

### Installation documentation must prove

- exact supported environment;
- expected installation time;
- required permissions and scheduled jobs;
- email and file-storage setup;
- production security checklist;
- backup and update procedure;
- how to diagnose failed jobs and mail; and
- what information to include in a support request without exposing secrets.

---

## 12. Marketplace launch plan

### Primary channel: CodeCanyon

CodeCanyon is selected because it already aggregates buyers for code and scripts, supports customer-set item prices, and strongly encourages functional live previews. Envato's current author terms are non-exclusive, so the same product can also be sold through a separate direct channel. [Envato author terms](https://help.author.envato.com/hc/en-us/articles/41371538488473-Envato-Market-Author-Terms)

The final marketplace category is chosen only after the senior engineer decides the implementation stack. Recalculate the fixed buyer fee and earnings before submission.

### Listing proposition

**Suggested title:**  
BriefRelay — Self-Hosted Client Approval & Project Handoff Portal

**Suggested one-line description:**  
A focused client portal for freelancers and agencies to share progress, collect feedback, approve deliverables, and complete project handoff—without a monthly fee.

**Honest search terms:**

- client portal
- client dashboard
- freelancer project management
- agency client portal
- deliverable approval
- project handoff
- file sharing
- client feedback
- invoice portal
- self-hosted portal

Use terms only where the shipped product genuinely satisfies the intent. Do not append unrelated popular technologies or “SaaS” claims merely for traffic.

### Listing proof assets

The plan intentionally gives no aesthetic direction. The listing still needs these proof artifacts:

- a short end-to-end product video;
- screenshots covering the owner and client journeys;
- a feature and scope comparison;
- performance benchmark summary with test conditions;
- security and privacy summary without unverifiable claims;
- installation steps and requirements;
- public documentation preview;
- version history; and
- direct access to the resettable demo.

### Product-led marketplace marketing

The first 100-sales strategy avoids dependence on a personal audience:

1. Use an exact, problem-led title and category.
2. Make the two-role live demo the primary proof.
3. Show the entire workflow in under two minutes.
4. State “one-time $29” and “self-hosted” in the first screen of listing copy.
5. Publish installation requirements before purchase to reduce refunds.
6. Answer marketplace questions within one business day during launch month.
7. Turn repeated presale questions into listing copy or documentation.
8. Publish small, meaningful fixes with a transparent changelog.
9. Ask satisfied buyers for an honest review; never buy, gate, or script reviews.
10. Keep the direct Stripe store operationally separate and do not use Envato pages or buyer communications to divert marketplace transactions.

### Optional direct channel

The direct product page may use the existing Stripe setup and the same $29 public price. It should be treated as a second channel, not as the source of the first audience.

Before direct sales, assign merchant responsibilities for:

- sales tax and VAT;
- invoicing;
- refunds and chargebacks;
- license delivery;
- download security;
- privacy notices;
- customer support records; and
- software export or sanctions restrictions where applicable.

---

## 13. Validation plan before full build

### Three-day validation sprint

#### Day 1 — Mine existing demand

- Review at least 50 recent comments and low-star reviews across 10 relevant CodeCanyon products.
- Record requested features, installation complaints, security concerns, missing documentation, and support failures.
- Count repeated problems; do not promote one-off requests into scope.

#### Day 2 — Interview buyers

Recruit 10 freelancers or agency owners from the target segments. Show only the workflow description and a neutral process prototype based on the supplied design.

Ask for evidence, not opinions:

- Describe the last project where approval or handoff became messy.
- Which tools and messages were involved?
- How much time did the confusion cost?
- Who needs access?
- What information is too sensitive for a hosted third party?
- Have you paid for a client portal, CRM, or project template before?
- What stopped you from continuing to use it?
- Would you install a self-hosted product, and what would make installation unacceptable?

#### Day 3 — Test offer and scope

- Show the exact one-line promise, $29 price, included features, exclusions, installation requirement, and support terms.
- Ask each participant to complete a forced ranking of the workflows.
- Invite qualified participants into a limited beta.

### Go/no-go gate

Proceed when all are true:

- at least 5 of 10 target users experienced the problem in the last three months;
- at least 4 currently combine three or more tools/messages for client delivery;
- at least 3 agree to install or actively test the beta;
- deliverable approval/handoff ranks in the top two jobs for at least 5; and
- no required feature forces the product into accounting, legal e-signature, or full CRM scope.

If the gate fails, do not start full implementation. Narrow the segment or problem and repeat the sprint.

---

## 14. Beta plan

### Beta cohort

- 8–12 target businesses.
- At least three professional segments.
- A mixture of solo operators and small teams.
- Real projects allowed only after the tester accepts that this is prerelease software and maintains their own backup.

### Beta success criteria

- 80% complete installation without a live call.
- Median setup time is 10 minutes or less on a supported environment.
- 70% create a client, project, and first deliverable in the first session.
- At least 10 real or realistic deliverables complete the share-to-decision flow.
- No cross-client data exposure or high-severity security issue.
- Fewer than three support contacts per tester during the first week.
- At least 5 testers state that the shipped scope is worth $29.
- At least 3 testers volunteer a truthful testimonial or marketplace review after becoming verified buyers; no review is required as a beta condition.

### Feedback triage

Classify every report as:

- launch blocker;
- documentation or setup friction;
- defect;
- usability issue;
- post-launch candidate; or
- outside product strategy.

Only launch blockers, serious friction, defects, and critical usability failures enter the version 1 sprint.

---

## 15. First 100-customer operating plan

### Planning funnel

The following are targets, not market facts:

| Stage | Target rate | Volume needed for 100 sales |
|---|---:|---:|
| Marketplace listing views | — | 4,000 |
| Listing to live-demo visit | 35% | 1,400 |
| Demo to purchase | 7.2% | 100 |
| Overall listing conversion | 2.5% | 100 |

Track marketplace views, demo sessions, role selected, completion of the approval flow, checkout starts where measurable, sales, refunds, support contacts, and review themes. Do not collect client content or invasive demo telemetry.

### Cumulative sales checkpoints

- Day 14 after approval: 10 sales.
- Day 30: 25 sales.
- Day 60: 60 sales.
- Day 90: 100 sales.

These checkpoints are management triggers, not promises.

### Diagnostic actions

| Signal | Likely problem | Action |
|---|---|---|
| Fewer than 500 listing views by day 30 | Discovery or marketplace-positioning problem | Revisit category, title, honest search terms, first-screen promise, and submission metadata |
| Listing views but demo rate below 25% | Weak relevance or proof | Clarify who it is for, what it replaces, self-hosting, one-time price, and demo path |
| Demo traffic but purchase conversion below 5% | Value, trust, installation, or scope objection | Review demo exits, presale questions, requirements, documentation, and competitor objections |
| Refund rate above 5% | Expectation or product-quality failure | Pause expansion, contact refund cases where permitted, fix mismatch, and update listing |
| More than 30 support minutes per sale after the first 20 sales | Product or documentation is too expensive to support | Remove recurring friction, improve diagnostics, and narrow supported environments |
| Repeated requests for the same missing core capability | Scope gap | Validate and schedule a focused update |
| Requests spread across unrelated CRM features | Wrong audience or positioning drift | Restate exclusions and protect the narrow workflow |

### Customer-success loop

1. Acknowledge valid support requests within one business day during launch month.
2. Reproduce against the exact customer version.
3. Publish a workaround or fix with a target date.
4. Add the resolution to the knowledge base.
5. Tag the underlying cause: code, docs, environment, expectation, or unsupported customization.
6. Review tags weekly and eliminate the highest-volume cause.

Envato's standard supported-item model includes six months of support, while updates required to keep the item working as described and protected from major security issues apply to all items. [Envato item support](https://help.market.envato.com/hc/en-us/articles/208191263-What-is-Item-Support)

---

## 16. Metrics dashboard

### Commercial

- paid licenses;
- gross customer spend;
- estimated marketplace earnings;
- direct-channel net receipts;
- listing-view-to-sale conversion;
- demo-to-sale conversion;
- refund and chargeback rate;
- review count, average rating, and recurring themes; and
- sales by marketplace search term where available.

### Activation

- successful installations;
- median time from installation start to owner dashboard;
- percentage creating a client and project in the first session;
- percentage sharing a first deliverable within 24 hours;
- invitation acceptance rate; and
- share-to-client-decision completion rate.

### Quality

- failed installation rate;
- open defects by severity;
- permission-test coverage;
- performance-budget trend;
- background-job failure rate;
- update success and rollback rate;
- restore-test success; and
- applicable dependency vulnerabilities by severity.

### Support economics

- contacts per sale;
- median first-response time;
- median resolution time;
- minutes of support per sale;
- percentage resolved by documentation; and
- top five recurring causes.

---

## 17. Risk register

| Risk | Probability | Impact | Prevention | Trigger and response |
|---|---|---|---|---|
| Scope expands into a full CRM | High | High | Enforce exclusions and critical path | Any phase slips by more than 20% because of added modules: remove additions immediately |
| Marketplace rejects the item | Medium | High | Follow current packaging, documentation, originality, and quality rules; reserve review buffer | Address specific reviewer feedback; do not add unrelated features reflexively |
| Buyers cannot install it | Medium | High | Preflight checker, one supported path, clean-install beta | More than 20% beta install failure: block launch |
| Cross-client data exposure | Low | Critical | Central authorization policies and permission matrix tests | Any confirmed case: stop release/demo, patch, rotate affected secrets, investigate scope |
| $29 creates excessive support load | Medium | High | Narrow support promise, diagnostics, self-serve docs | Over 30 minutes per sale after sale 20: repair onboarding or reduce supported matrix |
| One-time model creates indefinite obligations | High | Medium | Perpetual use, bounded support, disciplined compatibility policy | Publish end-of-support policy before a dependency reaches end of life |
| Marketplace fees make revenue disappointing | High | Medium | Separate gross and net goals; add independent direct channel later | Review economics at sales 25 and 100 |
| Competitors copy the feature set | High | Medium | Win on workflow quality, installation, documentation, security, and update reliability | Quarterly competitor audit; improve core workflow, not feature count |
| Demo is abused | Medium | Medium | Synthetic data, reset, caps, disabled outbound functions | Abuse spike: restrict functions or require temporary demo access |
| Name conflicts with an existing mark | Medium | High | Trademark, marketplace, company, app-store, and domain screening | Any credible conflict: rename before public beta |
| Buyer expects legal e-signature/accounting | Medium | High | Clear language and scope boundaries | Repeated confusion: revise listing and in-product copy before adding functionality |

---

## 18. Launch checklist

### Product

- [ ] Validation gate passed.
- [ ] Version 1 scope complete; exclusions remain excluded.
- [ ] Owner-to-client-to-sign-off journey passes from the customer archive.
- [ ] Sample data demonstrates the whole workflow.
- [ ] All permission invariants pass.
- [ ] Search, pagination, export, and deletion behave at benchmark scale.

### Security and operations

- [ ] Independent security review complete.
- [ ] No applicable high or critical vulnerability.
- [ ] No secrets or real personal data in the package or demo.
- [ ] Backup, restore, update, and rollback drills pass.
- [ ] Demo reset and anti-abuse controls pass.
- [ ] Health and diagnostics information is useful but non-sensitive.

### Performance and compatibility

- [ ] Published benchmark is reproducible.
- [ ] Performance budgets pass.
- [ ] Supported browser smoke tests pass.
- [ ] Keyboard and screen-reader smoke tests pass.
- [ ] File limits and failure handling pass.

### Package and legal

- [ ] Final name cleared for launch.
- [ ] All third-party assets and dependencies are licensed for redistribution.
- [ ] License notices and dependency inventory included.
- [ ] Installation, user, update, and troubleshooting documents verified by a new tester.
- [ ] Marketplace category, regular license, extended license, support status, and earnings recalculated under current terms.
- [ ] Privacy, terms, refund, and support statements reviewed.

### Marketplace

- [ ] Exact title and honest search terms prepared.
- [ ] Listing states self-hosting and requirements before purchase.
- [ ] Live demo works inside the marketplace preview environment.
- [ ] Owner and client credentials reset automatically.
- [ ] Short walkthrough shows the signature approval flow.
- [ ] Support inbox, response owner, and escalation path are ready.
- [ ] Release archive checksum matches the tested archive.

---

## 19. Post-launch roadmap rules

### Version 1.1 candidates

Only consider after evidence from at least 20 paid customers:

- reusable project presets;
- scheduled status-update reminders;
- expanded import/export;
- additional file-storage adapter;
- optional two-factor authentication; and
- one additional language supplied through the translation system.

### Version 1.2 candidates

Only consider after 50 paid customers and stable support economics:

- simple webhook events;
- configurable intake fields within strict limits;
- client organization with multiple project workspaces;
- richer owner reports; and
- documented advanced deployment profile.

### Never add merely to inflate the feature list

- payroll;
- inventory;
- full accounting;
- marketing automation;
- general-purpose CRM;
- AI wrappers without a validated job;
- native chat; or
- a multi-tenant hosted service under the same $29 promise.

Every roadmap candidate must pass four questions:

1. Did at least five paid customers independently request or reveal the need?
2. Does it improve onboarding, delivery, approval, or handoff?
3. Can it be supported without pushing support cost above the target?
4. Does it preserve self-hosted, one-time ownership without a creator-run dependency?

---

## 20. Definition of commercial success

The project succeeds when all of the following are true:

- 100 legitimate paid licenses have been sold at the $29 public list price;
- the refund rate is at or below 5%;
- the product has no unresolved high or critical security issue;
- at least 60% of measurable buyers who enable anonymous activation telemetry, or an equivalent beta sample, reach first deliverable share;
- support averages no more than 30 minutes per sale after the first 20 sales;
- the exact marketplace net proceeds are understood and acceptable;
- buyers can keep using their installed version without paying the creator again; and
- the product remains a focused client approval and handoff portal rather than a discounted full CRM.

The plan cannot guarantee 100 sales. It is designed to maximize the odds through a proven buyer category, a narrow gap, a low-friction price, a working demo, strict product scope, and measurable go/no-go gates.

---

## 21. Immediate next actions

1. Approve the product thesis and the $29 one-time model.
2. Complete the three-day validation sprint.
3. Clear or replace the working name.
4. Give the supplied design to the product owner for state and workflow mapping; make no aesthetic changes in this plan.
5. Have the senior engineer write the technology-selection decision record using the criteria in section 7.
6. Freeze the permission matrix, data rules, and approval state machine.
7. Build the installation/authentication foundation and the signature deliverable-approval workflow first.
8. Run the beta and enforce its launch gates.
9. Package and submit the exact tested archive to CodeCanyon.
10. Operate against the first-100-customer checkpoints and diagnostic actions.

---

## 22. Source register

All marketplace prices and sales counts are snapshots and must be rechecked immediately before pricing and submission.

- [Upwork — Future Workforce Index 2025](https://www.upwork.com/research/future-workforce-index-2025)
- [Upwork — Most In-Demand Skills for 2025](https://www.upwork.com/research/in-demand-skills-2025)
- [CodeCanyon — Project Management Tools](https://codecanyon.net/category/php-scripts/project-management-tools)
- [CodeCanyon — CRM Project Management Tools](https://codecanyon.net/category/php-scripts/project-management-tools?term=crm)
- [Envato — Client Portal Search](https://themeforest.net/search/client%20portal)
- [CodeCanyon — Client Dashboard Search](https://codecanyon.net/search/client%20dashboard)
- [ThemeForest — Admin Dashboard Templates](https://themeforest.net/category/site-templates/admin-templates)
- [Notion — CRM Template Marketplace](https://www.notion.com/en-gb/templates/category/crm)
- [Framer — Portfolio Templates](https://www.framer.com/templates/categories/portfolio/)
- [Framer — ClientHub](https://www.framer.com/marketplace/templates/clienthub/)
- [HandoffHQ](https://handoffhq.app/)
- [ClientDock](https://www.clientdock.pro/)
- [Envato — Author Terms](https://help.author.envato.com/hc/en-us/articles/41371538488473-Envato-Market-Author-Terms)
- [Envato — Introduction to Earnings](https://help.author.envato.com/hc/en-us/articles/360000472943-Introduction-to-Earnings)
- [Envato — Fixed Buyer Fees](https://help.author.envato.com/hc/en-us/articles/360000473203-Fixed-Buyer-Fees-on-Envato-Market)
- [Envato — Code Item Technical Requirements](https://help.author.envato.com/hc/en-us/articles/360000471583-Code-Item-Preparation-Technical-Requirements)
- [Envato — Item Presentation Requirements](https://help.author.envato.com/hc/en-us/articles/360000424863-Item-Presentation-Requirements)
- [Envato — Item Support](https://help.market.envato.com/hc/en-us/articles/208191263-What-is-Item-Support)
- [Envato — License Guide](https://help.market.envato.com/hc/en-us/articles/115005593363-Do-I-need-a-Regular-License-or-an-Extended-License)
- [Envato — License Duration](https://help.market.envato.com/hc/en-us/articles/115005597546-Do-licenses-have-an-expiry-date-Will-I-ever-have-to-pay-any-fees-like-renewals-or-royalties)
