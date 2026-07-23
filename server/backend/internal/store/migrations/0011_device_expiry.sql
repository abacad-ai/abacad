-- Per-device enrollment expiry (unix seconds; 0 = permanent, never expires).
--
-- On the hosted service, devices are ephemeral by default: they expire after a
-- TTL unless extended or explicitly made permanent (with attestation), which
-- bounds the standing blast radius of a relay compromise. Self-hosted instances
-- leave the TTL at 0, so this column stays 0 and enrollment never expires.
-- Existing devices default to 0 (permanent) — no behavior change on upgrade.
ALTER TABLE devices ADD COLUMN expires_at INTEGER NOT NULL DEFAULT 0;
