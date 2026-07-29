package store

import "strings"

// Activity is one row of the persistent account activity trail (the Activities
// page). Kind is a dotted "category.action" ("auth.login", "device.connected");
// the bare kind "command" is a relayed device command with Method/Source/Outcome
// filled in. ID is monotonic and doubles as the pagination cursor.
// ActorKind values for Activity.ActorKind — the class of credential behind a row.
// Orthogonal to Source, which records the channel the action arrived on: an
// ActorAPIKey can appear with source "agent" (a /mcp call) or "tunnel" (/connect).
const (
	ActorSession = "session" // dashboard browser session
	ActorAPIKey  = "apikey"  // scoped API key (bearer) on /mcp or /connect
	ActorDevice  = "device"  // the device itself, over its /device socket
	ActorSSH     = "ssh"     // SSH jump host, authenticated by public key
)

// maxUserAgent bounds the stored User-Agent. The header is client-controlled and
// unbounded on the wire; the trail only needs enough to tell a browser from a CLI.
const maxUserAgent = 256

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

	// Provenance: who caused this and from where. All may be empty — rows that
	// predate the actor migrations have none, and a few events legitimately have
	// no actor (a device self-registering has no account yet). ActorLabel is a
	// snapshot taken at write time, not a live join, so a revoked API key stays
	// attributable. See migrations 0015-0019.
	ActorKind  string
	ActorID    string
	ActorLabel string
	IP         string
	UserAgent  string

	// Where IP resolved to when the row was written, if a geo database was
	// configured. Country is an ISO 3166-1 alpha-2 code; City is often absent or
	// approximate and is only ever a hint. See migrations 0021-0022.
	Country string
	City    string
}

// InsertActivity appends one activity row. Ts must already be stamped.
func (s *Store) InsertActivity(a Activity) error {
	ua := a.UserAgent
	if len(ua) > maxUserAgent {
		// Cut on a byte boundary, then drop the partial rune the cut may have
		// left behind so the column never holds invalid UTF-8.
		ua = strings.ToValidUTF8(ua[:maxUserAgent], "")
	}
	_, err := s.db.Exec(
		`INSERT INTO activities(account_id,device_id,ts,kind,method,source,outcome,duration_ms,detail,
		                        actor_kind,actor_id,actor_label,ip,user_agent,country,city)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.AccountID, a.DeviceID, a.Ts, a.Kind, a.Method, a.Source, a.Outcome, a.DurationMs, a.Detail,
		a.ActorKind, a.ActorID, a.ActorLabel, a.IP, ua, a.Country, a.City)
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
	ActorID  string // exact credential, e.g. apikey_<random>
	IP       string // exact client address
	Country  string // ISO 3166-1 alpha-2, e.g. "GB"
	Limit    int
}

// Activities returns an account's trail, newest first.
func (s *Store) Activities(accountID string, f ActivityFilter) ([]Activity, error) {
	q := strings.Builder{}
	q.WriteString(`SELECT id,account_id,device_id,ts,kind,method,source,outcome,duration_ms,detail,
		       actor_kind,actor_id,actor_label,ip,user_agent,country,city
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
	if f.ActorID != "" {
		q.WriteString(` AND actor_id=?`)
		args = append(args, f.ActorID)
	}
	if f.IP != "" {
		q.WriteString(` AND ip=?`)
		args = append(args, f.IP)
	}
	if f.Country != "" {
		q.WriteString(` AND country=?`)
		args = append(args, f.Country)
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
			&a.Source, &a.Outcome, &a.DurationMs, &a.Detail,
			&a.ActorKind, &a.ActorID, &a.ActorLabel, &a.IP, &a.UserAgent,
			&a.Country, &a.City); err != nil {
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
