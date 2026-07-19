-- Drop the legacy "dev_" prefix from device ids. Device ids surface in URLs
-- (/devices/<id>), in ssh hostnames, and in the agent's device selection, so
-- they are now bare lowercase-base32 tokens with no type tag. New devices are
-- generated prefix-free (auth.NewDeviceID); this rewrites any rows created
-- before that so no environment is left with a mix of formats.
--
-- Idempotent: once stripped, no id matches 'dev_%', so re-running on each boot is
-- a no-op. The '_' is escaped to a literal (LIKE treats a bare '_' as a wildcard),
-- and base32 ids never contain '_', so this can only ever match the old prefix —
-- never a new-style id that merely begins with the letters "dev".
--
-- Safe without cascades: devices.id has no foreign-key referrers, the device
-- token lives in a separate column (token_hash) so connectivity is unaffected,
-- and activities.device_id is plain text rewritten here in lockstep.
UPDATE activities SET device_id = substr(device_id, 5) WHERE device_id LIKE 'dev\_%' ESCAPE '\';
UPDATE devices    SET id        = substr(id, 5)        WHERE id        LIKE 'dev\_%' ESCAPE '\';
