-- Add per-group Codex service_tier override.
-- follow_upstream: do not modify client request
-- force_fast: force OpenAI upstream priority tier (Codex fast)
-- disallow_fast: strip fast/priority tier from upstream request
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS codex_service_tier_mode VARCHAR(32) NOT NULL DEFAULT 'follow_upstream';

COMMENT ON COLUMN groups.codex_service_tier_mode IS 'Codex service_tier override mode: follow_upstream, force_fast, disallow_fast.';
