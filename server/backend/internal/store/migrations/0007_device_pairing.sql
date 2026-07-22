-- Device-authorization pairing (RFC 8628, à la `gh auth login`): a headless CLI
-- enrolls without copy-pasting a token. The CLI holds a secret device_code; the
-- human types the short user_code into /pair in their logged-in browser to
-- approve. On approval the row records the approving account and the chosen
-- name/platform; the CLI's next poll consumes the row, mints the device token
-- once (via the normal CreateDevice path), and the pairing is done.
--
-- No token is ever stored here — it is minted at consume time and returned to the
-- CLI exactly once, matching how createDevice treats device_token as "shown once".
-- Rows are short-lived (expires_at) and single-use (consumed), so a leaked or
-- abandoned code is harmless after its window.
--
-- Idempotent (CREATE TABLE IF NOT EXISTS), matching the re-run-every-boot
-- migration model — no version bookkeeping.
CREATE TABLE IF NOT EXISTS device_pairings (
  device_code TEXT PRIMARY KEY,                     -- secret, CLI-held (abd_pair_<random>)
  user_code   TEXT NOT NULL UNIQUE,                 -- short, human-typed (e.g. WXYZ-1234)
  status      TEXT NOT NULL DEFAULT 'pending',      -- pending | approved | denied
  account_id  TEXT REFERENCES accounts(id) ON DELETE CASCADE, -- NULL until approved
  name        TEXT NOT NULL DEFAULT '',             -- device name chosen by the approver
  platform    TEXT NOT NULL DEFAULT '',             -- device platform chosen by the approver
  created_at  INTEGER NOT NULL,
  expires_at  INTEGER NOT NULL,                     -- unix seconds; poll/approve refuse past this
  consumed    INTEGER NOT NULL DEFAULT 0            -- 1 once the CLI has fetched its token
);
CREATE INDEX IF NOT EXISTS idx_pairings_user_code ON device_pairings(user_code);
