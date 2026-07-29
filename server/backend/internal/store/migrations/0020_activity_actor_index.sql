-- Supports the "everything this credential did" query behind the Activities
-- page's actor filter: WHERE account_id = ? AND actor_id = ? ORDER BY id DESC.
-- The existing idx_activities_account_id_id still covers the unfiltered trail.
--
-- Genuinely idempotent (IF NOT EXISTS), so unlike the ADD COLUMN migrations this
-- one does not depend on the runner's duplicate-column skip. It must sort after
-- 0016, which creates the column it indexes.
CREATE INDEX IF NOT EXISTS idx_activities_account_actor ON activities(account_id, actor_id, id);
