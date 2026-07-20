-- Scoped API keys: replace the single full-access MCP token with named keys, each
-- restricted to a set of devices and methods (and optionally the /connect tunnel).
-- "All" is stored as a wildcard (all_devices=1 / methods='*'), which also covers
-- devices/methods created in the future — never a snapshot of the current set.

CREATE TABLE IF NOT EXISTS api_keys (
  id           TEXT PRIMARY KEY,             -- apikey_<random>
  account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,         -- sha-256 of the secret (auth.HashToken)
  all_devices  INTEGER NOT NULL DEFAULT 0,   -- 1 = every device incl. future ones
  methods      TEXT NOT NULL DEFAULT '*',    -- '*' = every verb incl. future ones; else CSV of verbs
  allow_tunnel INTEGER NOT NULL DEFAULT 0,   -- 1 = may open /connect
  created_at   INTEGER NOT NULL,
  last_used    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_api_keys_account ON api_keys(account_id);

-- Specific-device allowlist; consulted only when all_devices = 0. The FK cascades
-- on device deletion, so a removed device silently drops out of every key.
CREATE TABLE IF NOT EXISTS api_key_devices (
  api_key_id TEXT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
  device_id  TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  PRIMARY KEY (api_key_id, device_id)
);

-- Migrate the existing single MCP token into a full-access "Default" key so
-- established credentials keep working. Idempotent: only runs when the token
-- hasn't already been copied.
INSERT INTO api_keys (id, account_id, name, token_hash, all_devices, methods, allow_tunnel, created_at, last_used)
SELECT id, account_id, 'Default', token_hash, 1, '*', 1, created_at, last_used
FROM account_mcp_tokens t
WHERE NOT EXISTS (SELECT 1 FROM api_keys k WHERE k.token_hash = t.token_hash);
