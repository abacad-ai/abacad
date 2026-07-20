-- External login identities (OAuth / OIDC) linked to accounts. One row per
-- (provider, subject) — e.g. ("google", the Google user's stable `sub` id) — so a
-- returning user is recognized by their provider identity, not their email
-- (which can change). A single account may link several providers.
--
-- Passwordless: accounts created via a provider store an empty password_hash;
-- CheckPassword on an empty bcrypt hash always fails, so such accounts can only
-- sign in through their linked provider (never with a password) until one is set.
--
-- Idempotent (CREATE TABLE IF NOT EXISTS), matching the re-run-every-boot
-- migration model — no ALTER on the accounts table, no version bookkeeping.
CREATE TABLE IF NOT EXISTS account_identities (
  provider   TEXT NOT NULL,            -- "google"
  subject    TEXT NOT NULL,            -- provider's stable user id (Google 'sub')
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  email      TEXT NOT NULL DEFAULT '', -- email at link time (informational)
  created_at INTEGER NOT NULL,         -- unix seconds
  PRIMARY KEY (provider, subject)
);
CREATE INDEX IF NOT EXISTS idx_identities_account ON account_identities(account_id);
