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
3. It resolves the effective upstream baseline for that tag. If upstream `main`
   contains a verified `chore: sync VERSION to <version> [skip ci]` commit that
   only updates `backend/cmd/server/VERSION`, that commit is used as the
   baseline; otherwise the release tag commit is used.
4. If `upstream-release` already points at the effective baseline, the workflow
   exits.
5. Otherwise, the workflow creates `sync/upstream-<tag>` from the effective
   baseline and cherry-picks `upstream-release..fork` onto it with
   `--empty=drop`.
6. A successful replay opens or updates a draft preview PR from
   `sync/upstream-<tag>` to `fork`. Review the generated diff, but do not use
   the GitHub merge button.
7. If replay conflicts, the workflow opens or updates a conflict issue and
   leaves `fork` unchanged.
8. After review and passing checks, comment `/promote-fork` on the preview PR.
9. The promote workflow verifies commenter permission and checks, then
   force-with-lease rewrites `fork`, moves `upstream-release` to the promoted
   effective upstream baseline, closes the PR, and deletes the sync branch.
10. The Docker workflow publishes `ghcr.io/ghostflying/sub2api:fork` only when
   `fork` is updated.
11. Keep GitHub CLI pull request commands pinned to `${GITHUB_REPOSITORY}` so
    fork workflows do not accidentally target the upstream repository.
