# Docker publish promotion race

1. Remove the redundant `Docker Image` workflow dispatch from `Promote Fork`.
2. Let the atomic update of `fork` be the only automatic Docker publish trigger.
3. Remove the no-longer-needed `actions: write` permission and update the
   promotion message and maintenance documentation.
4. Validate the workflow syntax, push the fork-only fix to `fork`, and verify
   that the resulting `:fork` image contains the new fork revision and version.
