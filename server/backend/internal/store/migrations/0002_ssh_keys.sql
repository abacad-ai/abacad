-- Authorized SSH public keys, per account. These gate the SSH jump server
-- (/sshjump): a client connecting to <device>.<base-domain> authenticates to the
-- jump with one of these keys, which identifies the owning account. Routing then
-- only permits devices that account owns. Public keys are not secrets — we store
-- the full authorized_keys line and index by SHA256 fingerprint for O(1) lookup.
CREATE TABLE IF NOT EXISTS ssh_keys (
  id          TEXT PRIMARY KEY,            -- sshk_<random>
  account_id  TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name        TEXT NOT NULL DEFAULT '',    -- human label (e.g. "laptop")
  fingerprint TEXT NOT NULL UNIQUE,        -- ssh.FingerprintSHA256 (SHA256:...)
  public_key  TEXT NOT NULL,              -- normalized authorized_keys line
  created_at  INTEGER NOT NULL,
  last_used   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_ssh_keys_account ON ssh_keys(account_id);
