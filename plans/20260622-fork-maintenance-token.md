## Plan

- Use `FORK_MAINTENANCE_TOKEN` for fork-maintenance git pushes and GitHub CLI write operations that may touch workflow files.
- Keep read-only upstream release resolution on the default `GITHUB_TOKEN`.
- Validate the workflow YAML and embedded shell snippets, then trigger the sync workflow to confirm the preview branch push succeeds.
