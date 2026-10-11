# Account usage token labels

## Scope

Clarify the existing 5h and 7d model usage labels in both supported locales.
The input bucket excludes cache reads and writes for both providers; the cache
read bucket represents tokens read from cache. Token recording, totals, and
billing behavior remain unchanged.

## Implementation

1. Start a feature branch from the current production `fork` branch.
2. Rename the Chinese input label to `非缓存输入` and cache read label to
   `缓存读取`; use `Uncached input` and `Cache read` in English.
3. Run the existing locale completeness and model usage dialog checks, and
   inspect the diff for unintended changes.
4. Open a normal fork-only PR targeting `fork`, wait for CI and Security Scan
   on its exact head, and squash merge to preserve the linear patch stack.
5. Verify the production ref and the automatically triggered Docker publish.
   Docker publishing does not deploy the running service.
