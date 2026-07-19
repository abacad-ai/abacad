-- abacad schema. Minimal-but-real multi-tenancy: accounts own devices and one
-- MCP token; the web dashboard rides on sessions. All secret tokens are stored
-- hashed (sha-256 hex); the plaintext is shown to the user once, never at rest.

CREATE TABLE IF NOT EXISTS accounts (
  id            TEXT PRIMARY KEY,          -- acc_<random>
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,             -- bcrypt
  created_at    INTEGER NOT NULL           -- unix seconds
);

CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,             -- opaque cookie value (random)
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_account ON sessions(account_id);

CREATE TABLE IF NOT EXISTS devices (
  id         TEXT PRIMARY KEY,             -- dev_<random>; the selectable device_id
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  token_hash TEXT NOT NULL UNIQUE,         -- sha-256 of the device token
  platform   TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  last_seen  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_devices_account ON devices(account_id);

-- One MCP token per account (rotation replaces the row).
CREATE TABLE IF NOT EXISTS account_mcp_tokens (
  id         TEXT PRIMARY KEY,
  account_id TEXT NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,         -- sha-256 of the MCP token
  created_at INTEGER NOT NULL,
  last_used  INTEGER NOT NULL DEFAULT 0
);

-- Blobs: the data-plane store. Binary payloads (files, screenshots, media) never
-- ride the device WebSocket; they are uploaded/downloaded over HTTP /blobs and
-- referenced by id from control frames. The bytes live on disk under the blob
-- dir; only metadata lives here. Scoped to an account for authorization.
CREATE TABLE IF NOT EXISTS blobs (
  id           TEXT PRIMARY KEY,           -- blob_<random>
  account_id   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size         INTEGER NOT NULL,           -- bytes on disk
  sha256       TEXT NOT NULL,              -- hex, for integrity checks
  created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_blobs_account ON blobs(account_id);
