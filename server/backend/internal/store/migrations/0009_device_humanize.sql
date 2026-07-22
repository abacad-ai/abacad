-- Per-device toggle for human-like pointer motion. Default on (1): the client
-- injects a curved cursor approach, jittered landing, and human-scaled timing so
-- agent-driven input isn't scored as a bot. Users can turn it off per device.
ALTER TABLE devices ADD COLUMN humanize INTEGER NOT NULL DEFAULT 1;
