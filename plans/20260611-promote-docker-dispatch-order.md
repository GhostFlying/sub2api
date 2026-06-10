## Plan

- Move the `Docker Image` workflow dispatch immediately after the atomic promote
  push, before any PR cleanup or close operation can fail.
- Keep the dispatch failure fatal, because a promoted fork without Docker
  publishing is an incomplete promote.
- Make sync branch deletion and PR closing/commenting best-effort cleanup after
  the dispatch, and skip `gh pr close` when GitHub has already marked the PR
  closed or merged.
- Validate the workflow syntax after editing.
