-- Record the version a client reports when it dials the /device socket
-- (?version=<v>), so version skew across a fleet is visible in the dashboard and
-- list_devices. One monorepo number shared by server and every client; blank for
-- rows that predate this and for older clients that don't report one.
--
-- Not idempotent in the CREATE-TABLE-IF-NOT-EXISTS sense — SQLite has no
-- `ADD COLUMN IF NOT EXISTS` — but the migrate() runner treats the "duplicate
-- column name" error a re-run produces as a no-op, so re-running every boot stays
-- safe. Keep this file to the single ALTER so that skip is unambiguous.
ALTER TABLE devices ADD COLUMN version TEXT NOT NULL DEFAULT '';
