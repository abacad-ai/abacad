-- Session replay: the frame-by-frame record of what an agent did on a device.
--
-- Deliberately self-contained rather than a join against `activities`. The
-- activity recorder is fire-and-forget (activity.Recorder.Record returns no row
-- id, and drops rows outright when its buffer is full), so there is no reliable
-- key to correlate a frame with a trail row. Duplicating five small columns is
-- cheaper than inventing one, and it keeps a replay query to a single index seek.
--
-- account_id is not a foreign key, matching `activities` (0003): scoping is done
-- in the query, and rows are removed by age, by the per-device cap, or when the
-- device is deleted — never by a cascade.
--
-- frame_id is '' for steps that carried no image: only screenshot and composite
-- return pixels, so tap/click/swipe are recorded as frameless steps and drawn as
-- markers over the frame that precedes them.
--
-- marker holds NUMERIC COORDINATES ONLY for pointer verbs, e.g.
-- {"kind":"tap","x":540,"y":1180}. Typed text (input_text) and evaluated code
-- (execute) are never stored: the point is to see where the agent touched, not
-- to archive what it wrote.
CREATE TABLE IF NOT EXISTS replay_steps (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  account_id  TEXT NOT NULL,
  device_id   TEXT NOT NULL,
  ts          INTEGER NOT NULL,             -- unix millis
  method      TEXT NOT NULL,                -- protocol verb: screenshot, tap, click, ...
  source      TEXT NOT NULL DEFAULT '',     -- agent | dashboard | ssh | tunnel
  outcome     TEXT NOT NULL DEFAULT '',     -- ok | timeout | device_gone | denied | canceled | error
  duration_ms INTEGER NOT NULL DEFAULT 0,
  actor_label TEXT NOT NULL DEFAULT '',     -- snapshot of the credential name at write time
  frame_id    TEXT NOT NULL DEFAULT '',     -- '' when the step carried no image
  w           INTEGER NOT NULL DEFAULT 0,   -- frame width in pixels, for marker placement
  h           INTEGER NOT NULL DEFAULT 0,
  marker      TEXT NOT NULL DEFAULT ''      -- JSON pointer coordinates; see above
);

-- The replay query is always "one device, one time range, in order".
CREATE INDEX IF NOT EXISTS idx_replay_device_ts ON replay_steps(device_id, ts);
-- The retention sweep is "everything older than a cutoff", across devices.
CREATE INDEX IF NOT EXISTS idx_replay_ts ON replay_steps(ts);
