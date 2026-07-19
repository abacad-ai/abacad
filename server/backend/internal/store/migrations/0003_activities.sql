-- Account-wide activity trail: the persistent record of everything that happened
-- in a workspace — sign-ins, credential changes, device lifecycle, connections,
-- and every relayed command. Unlike the in-memory events ring (live debugging),
-- this survives restarts and backs the dashboard's Activities page.
--
-- account_id is not a foreign key on purpose: deleting an account (future) should
-- not silently erase its trail, and failed sign-ins are recorded before any
-- cascade semantics would apply. Rows are pruned by age instead (retention).
CREATE TABLE IF NOT EXISTS activities (
  id          INTEGER PRIMARY KEY AUTOINCREMENT, -- monotonic; the pagination cursor
  account_id  TEXT NOT NULL,
  device_id   TEXT NOT NULL DEFAULT '',          -- '' for account-level events
  ts          INTEGER NOT NULL,                  -- unix millis
  kind        TEXT NOT NULL,                     -- dotted category.action, e.g. auth.login, device.connected, command
  method      TEXT NOT NULL DEFAULT '',          -- relayed command method (kind=command)
  source      TEXT NOT NULL DEFAULT '',          -- agent | dashboard | ssh | tunnel
  outcome     TEXT NOT NULL DEFAULT '',          -- ok | failed | timeout | device_gone | canceled | error
  duration_ms INTEGER NOT NULL DEFAULT 0,
  detail      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_activities_account_id_id ON activities(account_id, id);
