# Repository Instructions

## Fork Maintenance Model

This repository is maintained as a generated fork of `Wei-Shaw/sub2api`.

The durable invariant is:

`fork = upstream-release effective baseline + fork-only patch stack`

Branch roles:

- `fork` is the production fork branch and Docker publishing source.
- `upstream-release` is the patch-stack anchor for the last promoted upstream
  effective baseline. It is not a continuously updated upstream mirror, not a
  development branch, and not a Docker publishing source.
- `sync/upstream-<tag>` branches are generated preview branches. They are built
  from a new upstream effective baseline plus the replayed fork-only patch
  stack.
- `main` is not the active fork maintenance branch unless the user explicitly
  says otherwise.

An upstream effective baseline is usually the latest upstream release tag
commit. If upstream later writes a verified single-file release metadata commit
such as `chore: sync VERSION to <version> [skip ci]` that only changes
`backend/cmd/server/VERSION`, use that commit as the effective baseline instead
of the raw tag commit.

## Normal Sync Flow

1. Resolve the latest upstream release tag.
2. Resolve the effective upstream baseline for that tag.
3. If `upstream-release` already points at that effective baseline, no sync is
   needed.
4. Otherwise, create or refresh `sync/upstream-<tag>` from the effective
   baseline.
5. Replay `upstream-release..fork` onto the sync branch in order with
   `git cherry-pick --empty=drop`.
6. If replay conflicts only in `backend/cmd/server/wire_gen.go`, regenerate
   Wire from the merged provider sources and continue the cherry-pick. Do not
   use this recovery path when any other file conflicts.
7. If replay succeeds, push the sync branch and open or update a draft preview
   PR from `sync/upstream-<tag>` to `fork`.
8. Dispatch the preview CI and security workflows for the sync branch.
9. After review and passing checks, promote by commenting `/promote-fork` on the
   preview PR.
10. The promote workflow must atomically rewrite `fork` to the reviewed sync
   branch and move `upstream-release` to the promoted effective baseline.
11. Docker publishing runs from the updated `fork` branch.

Do not use the GitHub merge button for generated sync PRs.

## Conflict SOP

When the sync workflow reports a replay conflict:

1. Treat the failure as a blocked generated sync, not as a CI/test failure.
2. Keep both `fork` and `upstream-release` unchanged until the conflict is
   resolved and promoted.
3. Use the conflict issue or workflow log to identify the upstream tag,
   effective baseline, failed commit, and conflicted files.
4. Recreate or continue `sync/upstream-<tag>` from the reported effective
   baseline.
5. Cherry-pick `upstream-release..fork` in order with `--empty=drop`.
6. Resolve the conflicting commit by preserving the intended fork behavior while
   incorporating upstream changes.
   If `backend/cmd/server/wire_gen.go` is the only conflict, regenerate it with
   `go generate ./cmd/server` from `backend/` instead of editing generated code.
7. Continue the cherry-pick, then replay the remaining fork-only commits.
8. Add any durable fork-maintenance fixes as new commits on the sync branch so
   they become part of the fork-only patch stack after promotion.
9. Run the relevant local validation for the changed area.
10. Push `sync/upstream-<tag>` and let the preview PR/check workflow run.
11. After review and green required checks, use `/promote-fork`; promotion is the
    step that advances `upstream-release`.

If replay conflicts, do not manually advance `upstream-release` by itself. Doing
so breaks the patch-stack boundary and makes future replays ambiguous.
