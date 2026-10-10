# Account model token statistics (5h / 7d)

Implement the approved plan for Claude OAuth/Setup Token and Codex OAuth,
including Spark shadow accounts. Reuse this checkout and branch from fork.

- Add a read-only admin usage-model-stats endpoint, grouping upstream model
  (fallback model), returning four token buckets, totals and explicit boundaries.
- Share validated window resolution with compact totals. Prefer Claude session
  boundaries for 5h; otherwise valid sampled resets. Fall back to rolling 5h/7d.
- Query [start, end); cache snapshots for 60s and singleflight concurrent loads.
  Read only account state and logs: no probes, quota requests or state writes.
- Add lazy BaseDialog details on account rows, with 5h/7d tabs, desktop table,
  mobile cards, loading/empty/error/retry states, stale-response protection,
  focus restoration, translated labels and matching local totals.
- Cover SQL grouping/bounds, resolver, caches/errors/passivity, frontend states
  and responsive rendering. Run Go tests, Vitest, typecheck and i18n checks.
- No migrations, reasoning-token fields, specialized quota bars, merge or deploy.

## Validation evidence

- CI follow-up: check cache and singleflight result type assertions, assert the
  test cache entry type, rerun focused Go tests and verify the updated PR checks.

- Focused service/repository/admin-handler tests pass.
- Real PostgreSQL integration verifies grouped upstream/fallback models,
  account isolation, [start,end) bounds, four buckets and independent costs.
- Frontend component tests, typecheck, locale completeness and targeted ESLint.
- Playwright fixture at localhost:5198: desktop 1280x900, mobile 390x844,
  dark mode, lazy requests, tabs, unknown quota, Escape and focus restoration.
  Screenshots and browser report are outside the checkout in /tmp/sub2api-model-qa.
