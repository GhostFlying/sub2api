# Fork maintenance plan

This fork is maintained from a dedicated `fork` branch.

1. Create the `fork` branch from the latest upstream `origin/main`.
2. Keep fork-only commits on `fork`.
3. Add a scheduled workflow that detects new upstream releases and opens a pull
   request into `fork` instead of pushing directly.
4. Add a Docker workflow that publishes GHCR images when `fork` is updated.
5. Leave deployment docs unchanged; production compose files can point at
   `ghcr.io/ghostflying/sub2api:fork`.
