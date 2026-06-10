# Hourly Upstream Sync Schedule Plan

Goal: increase the automated upstream release detection cadence from every six
hours to every hour.

Implementation:
- Update `.github/workflows/sync-upstream-release.yml` schedule from
  `17 */6 * * *` to `17 * * * *`.
- Keep the existing minute offset (`17`) to avoid top-of-hour congestion.
- Validate the workflow YAML after the edit.
