-- Device self-registration: the pre-account holding pen.
--
-- A GUI client that has just been installed has nobody to belong to yet. It
-- registers itself here, displays its id and a short claim code on its own
-- screen, and waits for a human to claim it into an account at /claim.
--
-- Why a separate table instead of making devices.account_id nullable: an
-- unclaimed row must be UNREACHABLE by every account-scoped read (DeviceOwnedBy,
-- DevicesByAccount, the MCP resolver, VNC, blobs, devicegc). Keeping it out of
-- `devices` entirely makes that structural rather than a filter each of those
-- call sites has to remember. It also preserves the invariant that every row in
-- `devices` has an accountable owner, which the audit trail and the
-- non-escalation rule in docs/trust.md both rest on.
--
-- Unlike device_pairings (0007), this table pre-allocates the FINAL id and
-- token_hash. The human reads the device id off the device's screen before
-- claiming, so claiming must graduate that exact identity into `devices` —
-- re-minting would change the id out from under them and force the client to
-- re-key mid-flow. ClaimRegistration therefore copies id and token_hash across
-- unchanged and deletes the registration.
--
-- The claim code is separate from the id on purpose: two independent secrets,
-- both of which appear only on the device's own screen. The code is short-lived
-- and rotates; the id is not a secret once claimed.
CREATE TABLE IF NOT EXISTS device_registrations (
  id               TEXT PRIMARY KEY,           -- the FINAL device id (auth.NewDeviceID), reserved here
  token_hash       TEXT NOT NULL UNIQUE,       -- the FINAL device token hash; survives the claim unchanged
  claim_code       TEXT NOT NULL,              -- current rotating code, canonical XXXX-XXXX
  claim_expires_at INTEGER NOT NULL,           -- unix seconds; 0 once burned by a successful claim
  claim_attempts   INTEGER NOT NULL DEFAULT 0, -- failed preview guesses against the CURRENT code
  name             TEXT NOT NULL DEFAULT '',   -- client-suggested hostname
  platform         TEXT NOT NULL DEFAULT '',
  version          TEXT NOT NULL DEFAULT '',
  created_at       INTEGER NOT NULL,
  last_seen        INTEGER NOT NULL DEFAULT 0, -- heartbeat; drives "online" in the preview and idle GC
  reg_ip           TEXT NOT NULL DEFAULT ''    -- per-IP quota accounting and abuse forensics
);
CREATE INDEX IF NOT EXISTS idx_registrations_last_seen ON device_registrations(last_seen);
CREATE INDEX IF NOT EXISTS idx_registrations_reg_ip ON device_registrations(reg_ip);
