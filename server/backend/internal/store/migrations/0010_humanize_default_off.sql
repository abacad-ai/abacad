-- Flip humanize to OFF by default and reset existing devices.
--
-- "Humanize" synthesizes human-like pointer motion. Because it can be used to
-- make automated input statistically indistinguishable from a human's, it is now
-- an explicit, per-device opt-in that requires the operator to attest they own or
-- are authorized to automate the device and that the automation does not violate
-- the target platform's terms (see api.updateDevice). New devices default to 0
-- (store.CreateDevice); this migration resets every existing device to 0 so the
-- opt-in is uniform and re-attestation is required after upgrade.
UPDATE devices SET humanize = 0;
