# Fork maintenance plan

This fork is maintained as a generated `fork` branch:

`fork = upstream-release effective baseline + fork-only patch stack`

The `upstream-release` branch is the effective upstream release baseline that
the current `fork` branch is based on. It normally points at an upstream release
tag commit. If the upstream release workflow writes follow-up release metadata,
such as `backend/cmd/server/VERSION`, it may point at that verified upstream
metadata commit instead. It is only a patch-stack boundary, not a development
branch and not a Docker publishing source.

1. Keep fork-only commits directly on `fork`.
2. The scheduled sync workflow resolves the latest upstream release tag.
   Sync and promotion share one concurrency group so they cannot act on the
   maintenance refs at the same time.
3. It resolves the effective upstream baseline for that tag. If upstream `main`
   contains a verified `chore: sync VERSION to <version> [skip ci]` commit that
   only updates `backend/cmd/server/VERSION`, that commit is used as the
   baseline; otherwise the release tag commit is used.
4. If `upstream-release` already points at the effective baseline, the workflow
   skips replay and closes any stale conflict report for the promoted release.
5. If an open preview PR already targets the current `fork` head and its sync
   branch contains the effective baseline, the workflow keeps that reviewed
   preview instead of replaying the old patch stack again.
6. Otherwise, the workflow creates `sync/upstream-<tag>` from the effective
   baseline and cherry-picks `upstream-release..fork` onto it with
   `--empty=drop`.
7. If a cherry-pick conflicts only in `backend/cmd/server/wire_gen.go`, the
   workflow regenerates Wire from the merged provider sources and continues.
   Any other conflict still requires manual resolution.
8. A successful replay opens or updates a draft preview PR from
   `sync/upstream-<tag>` to `fork`. Review the generated diff, but do not use
   the GitHub merge button.
9. If replay conflicts, the workflow opens or updates a conflict issue and
   leaves `fork` unchanged. Before publishing the conflict, it verifies that
   the fetched `fork` and `upstream-release` refs have not moved.
10. After review and passing checks, comment `/promote-fork` on the preview PR.
11. The promote workflow verifies commenter permission and checks, then
   force-with-lease rewrites `fork`, moves `upstream-release` to the promoted
    effective upstream baseline, closes stale conflict reports, closes the PR,
    and deletes the sync branch.
12. The Docker workflow publishes `ghcr.io/ghostflying/sub2api:fork` only when
   `fork` is updated.
13. Keep GitHub CLI pull request commands pinned to `${GITHUB_REPOSITORY}` so
    fork workflows do not accidentally target the upstream repository.
