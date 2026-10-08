# Fork maintenance plan

This fork follows the candidate, checks, and automatic finalizer model used by
[GhostFlying/orca](https://github.com/GhostFlying/orca). Its invariant remains:

`fork = upstream-release effective baseline + fork-only patch stack`

`fork` is the production branch and Docker publishing source. `upstream-release`
is the effective upstream baseline of that branch, usually a published stable
release commit or its verified single-file VERSION metadata follow-up. It is a
patch-stack boundary, not a development branch or a continuously updated mirror.

## Automatic upstream releases

1. Keep fork-only changes as linear commits on `fork`; merge commits cannot be
   replayed by this maintenance flow.
2. The hourly sync selects the highest published stable `vX.Y.Z` upstream
   Release. Drafts, prereleases, and other tag formats are ignored. An explicit
   `upstream_tag` dispatch must satisfy the same eligibility rules. When polling
   skips multiple releases, only the highest eligible version is generated.
3. Resolve the effective baseline. A verified `chore: sync VERSION to <version>
   [skip ci]` commit that changes only `backend/cmd/server/VERSION` and contains
   the selected version may replace the tag commit. If already anchored, stop.
4. Capture the exact source `fork` and `upstream-release` SHAs, replay
   `upstream-release..fork` onto the effective baseline with
   `git cherry-pick --empty=drop`, and publish `sync/upstream-<tag>` with leases.
   Sync and promotion share one concurrency group. Source refs must still match
   before publishing a candidate.
5. Open or refresh a draft audit PR targeting `fork`. Its `fork-sync-source`
   version-2 marker records the source refs, effective baseline, tag commit, and candidate head.
   Reuse a current candidate and its checks instead of replaying it each hour.
   **Do not use the GitHub merge button for generated sync PRs.**
6. Ensure CI and Security Scan run against the exact candidate SHA. Queue the
   automatic finalizer for new and reused candidates. Completed candidate push
   or workflow_dispatch runs of CI / Security Scan also trigger the finalizer;
   no manual prompt, review approval, or `/promote-fork` comment is required.
7. The finalizer checks out trusted production `fork`, never candidate code. It
   accepts only same-repository stable release candidates targeting `fork`,
   matching source markers and the event's exact SHA. Only the latest successful
   GitHub Actions result at that SHA satisfies each required check: `test`,
   `frontend`, `golangci-lint`, `fork-maintenance`, `backend-security`, and
   `frontend-security`. Missing or running checks wait for a later completion;
   failures, cancellations, skipped checks, and neutral results block promotion.
8. Revalidate the published Release, exact tag commit, candidate ref, source refs, effective baseline, and baseline
   ancestry. Atomically force-with-lease update `fork` and `upstream-release`,
   also leasing the candidate ref. Repeated events are harmless; an already
   promoted transaction only completes cleanup and does not update refs again.
9. Read back production refs, close the audit PR and resolved conflict issues,
   then delete the candidate with its exact lease. A moved candidate must never
   be deleted by an older transaction.
10. The `fork` push triggers the existing Docker workflow. Never dispatch a
    second Docker run from promotion. This does not deploy a running service.

The required security checks remain required. Existing dependency findings or
expired audit exceptions can block a release; automatic promotion does not add
exceptions or bypass failed checks. Diagnose and fix a failed check, then rerun
that check. A completed rerun invokes the finalizer automatically. The Promote
Fork workflow also accepts a PR number (and optional exact head SHA) via
workflow_dispatch on `fork` for recovery; ordinary sync requires no manual use.

## Conflict recovery

Replay conflicts leave production refs unchanged. Only a conflict isolated to
`backend/cmd/server/wire_gen.go` may be recovered by `go generate ./cmd/server`;
all other conflicts require semantic resolution against the selected release.

1. Read the conflict issue/workflow and capture the source refs, selected tag,
   effective baseline, failed commit, and conflicted files.
2. Recreate the candidate from that baseline and replay the entire source patch
   range in order, retaining the intended fork behavior. Do not manually advance
   the anchor or merge upstream `main` into production.
3. Resolve the conflict, regenerate Wire when applicable, and continue replay.
   Add durable maintenance fixes as fork-only commits in the candidate.
4. Push the repaired candidate with its lease and refresh the audit PR marker to
   bind the unchanged source refs, baseline, and repaired head. Run local checks
   and dispatch CI / Security Scan at that exact candidate.
5. Passing checks automatically invoke the same finalizer. No manual promotion
   comment is needed. If a source ref moved, regenerate the candidate instead.

Before changing this flow run:

```sh
python -m unittest discover -s .github/scripts -p 'test_fork_*.py'
git diff --check
```

GitHub CLI commands in workflows must always specify `${GITHUB_REPOSITORY}` so
fork maintenance cannot accidentally target the upstream repository.
