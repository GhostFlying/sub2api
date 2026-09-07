# Codex model catalog multi-agent metadata

## Context

Sub2API locally synthesizes a Codex `/models` manifest when an OpenAI group
uses explicit model mappings. The synthesized entries currently omit the
multi-agent metadata used by Codex to select V1 or V2, so known models can
silently fall back to V1.

The fork maintenance invariant is:

`fork = upstream-release effective baseline + fork-only patch stack`

This work is based on `fork/fork` at `41357ba523f152f98105d6a7fe1b573a72bdec9d`.
It must not move `fork/upstream-release`, currently
`ab99d56e9626e6cd731592dae8553c9758a0efa2`.

## Plan

1. Backport upstream commits `2db78bd3` and `ad4b2f30`, which preserve
   `multi_agent_version` and `multi_agent_reasoning_effort` through upstream
   model synchronization, aliases, and group-level capability intersection.
2. Add local fallback metadata matching the verified Codex bundled catalog:
   Astra, Sol, and Terra use V2; Luna uses V1; only Astra maps Ultra to xhigh.
3. Keep explicit upstream values authoritative and preserve the existing
   fail-closed behavior when routed accounts disagree or omit metadata.
4. Do not persist or forward arbitrary `model_messages.multi_agent` prompts.
   Codex supplies bundled V2 instructions when the catalog prompt is absent.
5. Add service and HTTP-handler tests for exact model values, aliases, account
   intersection, explicit nulls, the configured-model early-return path, and
   final-body ETag behavior.
6. Run formatting, focused backend tests, unit tests, integration tests, and
   `git diff --check`.

## Delivery

Keep the upstream backport commits separate from the fork-specific completion
commit so future upstream syncs can drop commits that have become empty. Open a
normal PR targeting `fork`; do not move `upstream-release` and do not use the
generated sync-preview promotion flow.
