# Performance budgets and reference results

Budgets come from the plan (§6.1): authenticated reads p95 ≤ 300 ms, writes p95 ≤ 500 ms, search first page ≤ 500 ms,
measured against a seeded data set of 500 clients, 5,000 projects, 25,000 deliverable versions and 100,000 activity events.

## How to reproduce

```bash
make perf
```

This runs `TestPerformanceBudgets` in `internal/web/perf_test.go`. It seeds the data set into a temporary
database (about 2 s), then fires 40 requests per page through the real HTTP stack and reports the p95.
The test fails when any page is over budget, so it doubles as the regression gate: run it before every release
and compare with the table below. A p95 that is more than 15 % worse than the previous release needs an explanation
in the changelog.

## Reference environment

- Laptop, Intel Core i5-1135G7 (4 cores / 8 threads), NVMe SSD, WSL2 Linux 6.18
- Go 1.27.1, `modernc.org/sqlite` (pure Go), WAL mode, single process
- Loopback network (no transit), no reverse proxy

## Results, 2026-09-03 (commit after Phase 5)

| Page | p95 | median | budget |
|---|---:|---:|---:|
| GET /projects (list, 25 per page) | 20 ms | 7 ms | 300 ms |
| GET /projects?q= (search) | 11 ms | 9 ms | 500 ms |
| GET /clients | 2 ms | 1 ms | 300 ms |
| GET /clients?q= (search) | 2 ms | 1 ms | 500 ms |
| GET /projects/{id} | 3 ms | 1 ms | 300 ms |
| GET /clients/{id} | 1 ms | 1 ms | 300 ms |
| GET /deliverables/{id} | 2 ms | 1 ms | 300 ms |
| GET /activity (100k events) | 1 ms | 1 ms | 300 ms |
| GET / (dashboard) | 25 ms | 24 ms | 300 ms |
| GET /portal/projects/{id} | 2 ms | 1 ms | 300 ms |
| GET /portal/deliverables/{id} | 2 ms | 1 ms | 300 ms |
| POST milestone (write) | 1 ms | 1 ms | 500 ms |
| POST client comment (write) | 1 ms | 1 ms | 500 ms |

The benchmark found one regression during Phase 5: the activity page took 430 ms because SQLite used the
workspace index for an `OR` filter and then sorted all 100,000 rows. Forcing a newest-first rowid walk fixed it.
Project activity uses an expression index on the project id inside the event metadata (migration 0004).

## Rules that keep it this way

- No list view fetches an unbounded collection: every list is paginated with `LIMIT` (25 per page).
- Email and cleanup work goes through the job queue, never inside a request.
- Uploads stream to disk with a size cap; downloads stream from disk.
- One SQLite writer connection; readers use a small pool. If write contention ever shows up in `/healthz`
  job stats, that is the point to consider Postgres, not before.
