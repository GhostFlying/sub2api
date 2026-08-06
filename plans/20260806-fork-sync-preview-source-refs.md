# Fork sync preview source-ref validation

## Problem

An open sync preview can outlive the `fork` and `upstream-release` refs it was
generated from. GitHub updates a pull request's current base OID when `fork`
moves, so checking only `baseRefOid` does not prove that the preview contains
the latest fork-only patch stack. A successful replay can also publish a stale
preview if either source ref moves while the workflow is replaying commits.

## Plan

1. Add a machine-readable marker to generated preview pull request bodies with
   the source `fork`, source `upstream-release`, effective baseline, and
   generated head SHAs.
2. Treat an existing preview as active only when the marker matches the current
   source refs, effective baseline, and preview head.
3. Re-fetch and compare `fork` and `upstream-release` after a successful replay,
   then publish the generated preview with atomic source-ref and preview-ref
   leases.
4. Require promotion to validate the marker against the fetched source refs,
   recomputed effective baseline, and reviewed preview head before the atomic
   push.
5. Document the marker requirement for manually recovered conflict previews.

## Validation

- Parse both workflow YAML files.
- Run `bash -n` on every workflow `run` block.
- Exercise marker parsing with matching, stale-source, stale-baseline, and
  stale-head fixtures.
- Replay the complete fork-only patch stack onto upstream `main`.
- Run `git diff --check` and verify no temporary bootstrap workflows exist.
