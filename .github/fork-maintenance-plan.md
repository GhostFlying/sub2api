# Fork maintenance plan

This fork is maintained as a generated `fork` branch:

`fork = upstream-release baseline + fork-only patch stack`

The `upstream-release` branch is the upstream release commit that the current
`fork` branch is based on. It is only a patch-stack boundary, not a development
branch and not a Docker publishing source.

1. Keep fork-only commits directly on `fork`.
2. The scheduled sync workflow resolves the latest upstream release tag.
3. If `upstream-release` already points at that tag, the workflow exits.
4. Otherwise, the workflow creates `sync/upstream-<tag>` from the upstream tag
   and cherry-picks `upstream-release..fork` onto it with `--empty=drop`.
5. A successful replay opens or updates a draft preview PR from
   `sync/upstream-<tag>` to `fork`. Review the generated diff, but do not use
   the GitHub merge button.
6. If replay conflicts, the workflow opens or updates a conflict issue and
   leaves `fork` unchanged.
7. After review and passing checks, comment `/promote-fork` on the preview PR.
8. The promote workflow verifies commenter permission and checks, then
   force-with-lease rewrites `fork`, moves `upstream-release` to the promoted
   upstream tag, closes the PR, and deletes the sync branch.
9. The Docker workflow publishes `ghcr.io/ghostflying/sub2api:fork` only when
   `fork` is updated.
10. Keep GitHub CLI pull request commands pinned to `${GITHUB_REPOSITORY}` so
    fork workflows do not accidentally target the upstream repository.
