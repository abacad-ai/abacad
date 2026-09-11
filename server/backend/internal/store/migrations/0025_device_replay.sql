-- Per-device session replay, OFF by default. Recording every frame an agent sees
-- is screen content at rest — a materially larger data posture than the single
-- cached last-screenshot — so it takes a deliberate human act in the dashboard
-- (with attestation) to turn on, and the default must be off for every device
-- that already exists as well as every device created later.
ALTER TABLE devices ADD COLUMN replay INTEGER NOT NULL DEFAULT 0;
