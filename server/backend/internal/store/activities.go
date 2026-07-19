package store

import "strings"

// Activity is one row of the persistent account activity trail (the Activities
// page). Kind is a dotted "category.action" ("auth.login", "device.connected");
// the bare kind "command" is a relayed device command with Method/Source/Outcome
// filled in. ID is monotonic and doubles as the pagination cursor.
type Activity struct {
	ID         int64
	AccountID  string
	DeviceID   string
	Ts         int64 // unix millis
	Kind       string
	Method     string
	Source     string
	Outcome    string
	DurationMs int64
	Detail     string
}

// InsertActivity appends one activity row. Ts must already be stamped.
func (s *Store) InsertActivity(a Activity) error {
	_, err := s.db.Exec(
		`INSERT INTO activities(account_id,device_id,ts,kind,method,source,outcome,duration_ms,detail)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		a.AccountID, a.DeviceID, a.Ts, a.Kind, a.Method, a.Source, a.Outcome, a.DurationMs, a.Detail)
	return err
}

// ActivityFilter narrows an Activities query. Zero values mean "no filter".
// Kind matches the exact kind or the "category." prefix, so Kind="device"
// matches both device.connected and device.created.
type ActivityFilter struct {
	BeforeID int64 // return rows with id < BeforeID (0 = from the newest)
	DeviceID string
	Kind     string
	Source   string
	Limit    int
}

// Activities returns an account's trail, newest first.
func (s *Store) Activities(accountID string, f ActivityFilter) ([]Activity, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT id,account_id,device_id,ts,kind,method,source,outcome,duration_ms,detail
		FROM activities WHERE account_id=?`)
	args := []any{accountID}
	if f.BeforeID > 0 {
		q.WriteString(` AND id<?`)
		args = append(args, f.BeforeID)
	}
	if f.DeviceID != "" {
		q.WriteString(` AND device_id=?`)
		args = append(args, f.DeviceID)
	}
	if f.Kind != "" {
		q.WriteString(` AND (kind=? OR kind LIKE ?)`)
		args = append(args, f.Kind, f.Kind+".%")
	}
	if f.Source != "" {
		q.WriteString(` AND source=?`)
		args = append(args, f.Source)
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q.WriteString(` ORDER BY id DESC LIMIT ?`)
	args = append(args, limit)

	rows, err := s.db.Query(q.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.AccountID, &a.DeviceID, &a.Ts, &a.Kind, &a.Method,
			&a.Source, &a.Outcome, &a.DurationMs, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PruneActivities deletes rows older than beforeTs (unix millis) and reports how
// many were removed.
func (s *Store) PruneActivities(beforeTs int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM activities WHERE ts<?`, beforeTs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
