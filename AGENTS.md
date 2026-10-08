# Repository Instructions

## Fork Maintenance Model

This repository is maintained as a generated fork of `Wei-Shaw/sub2api`.
Before changing maintenance workflows, refs, or resolving replay conflicts, read
[`.github/fork-maintenance-plan.md`](.github/fork-maintenance-plan.md) in full.
The workflow and `.github/scripts/fork_promotion.py` are the executable policy.

The durable invariant is:

`fork = upstream-release effective baseline + fork-only patch stack`

- `fork` is the production branch and Docker publishing source.
- `upstream-release` anchors the last promoted effective upstream baseline. It
  is neither a development branch nor a continuously updated upstream mirror.
- `sync/upstream-<tag>` branches are generated candidates containing the selected
  stable upstream baseline and the replayed fork-only patch stack.
- `main` is not the active fork maintenance branch.

## Automatic Sync and Promotion

1. Select the highest published stable `vX.Y.Z` Release, ignoring drafts and
   prereleases. A manual tag dispatch follows the same eligibility rules.
2. Resolve the effective baseline: the release tag commit or a verified VERSION
   follow-up changing only `backend/cmd/server/VERSION` to the selected version.
3. Capture source refs and replay `upstream-release..fork` in order with
   `git cherry-pick --empty=drop`. Keep the fork stack linear.
4. Publish the leased candidate and draft audit PR with its exact
   `fork-sync-source` marker. Reuse a current preview; do not rebuild it hourly.
5. CI and Security Scan completions automatically invoke promotion. Required
   checks must succeed on the exact candidate SHA; missing/running checks wait,
   failed/skipped checks block. No manual prompt or `/promote-fork` is required.
6. The trusted finalizer verifies source refs, candidate SHA, effective baseline,
   and ancestry, then atomically updates `fork` and `upstream-release` with
   force-with-lease. Repeated completion events must be idempotent.
7. Read back refs, close the audit PR and resolved issues, and delete the
   candidate only with its exact lease.
8. The `fork` push triggers Docker publishing. Do not dispatch an additional
   Docker workflow or claim that publishing constitutes deployment.

Do not use the GitHub merge button for generated sync PRs. Normal fork-only
feature PRs may be squash merged to keep the patch stack linear. Promotion may
be retried through workflow_dispatch on `fork`; this is a recovery entry, not a
required approval step. Do not bypass failed security checks or extend exceptions
merely to make automatic promotion proceed.

## Conflict SOP

1. Treat replay conflicts as a blocked sync, leaving production refs unchanged.
2. Identify the selected release, captured source refs, baseline, failed commit,
   and conflicted files from the issue or workflow log.
3. Recreate the candidate from the effective baseline and replay the captured
   patch range in order, retaining intended fork behavior against upstream.
4. If only `backend/cmd/server/wire_gen.go` conflicts, run
   `go generate ./cmd/server` from `backend/`; do not hand-edit generated Wire.
   Other conflicts require semantic resolution.
5. Finish replay, add durable maintenance fixes, run local checks, and publish a
   leased repaired candidate with a refreshed source-ref marker.
6. Run CI and Security Scan at that exact SHA; the automatic finalizer promotes
   once the gates pass. If source refs moved, regenerate the preview.

Never advance `upstream-release` independently or substitute a merge from
upstream `main` for candidate generation and atomic promotion.
