package store

import (
	"database/sql"
	"errors"
)

// Session replay persistence: one row per relayed command on a device with
// replay turned on, carrying a reference to the frame the device returned (when
// it returned one). See migrations/0024_replay_steps.sql for why the row is
// self-contained rather than a join against `activities`.
//
// Every delete path here returns the frame ids it removed so the caller can
// delete the matching files. The rows and the bytes live in different places
// (SQLite and the replay directory), and this is the only ordering that cannot
// leak: rows first, then files. The reverse would leave a row pointing at a
// frame that is already gone.

// ReplayStep is one recorded command. FrameID is "" when the command returned no
// image — only screenshot and composite do — and such a step is drawn as a
// marker over the frame before it rather than as a frame of its own.
type ReplayStep struct {
	ID         int64
	AccountID  string
	DeviceID   string
	Ts         int64 // unix millis
	Method     string
	Source     string
	Outcome    string
	DurationMs int64
	ActorLabel string
	FrameID    string
	W, H       int
	Marker     string // JSON pointer coordinates, or "" — never text or code
}

// maxReplayRange bounds one ReplaySteps query. A session is tens to hundreds of
// steps; this is a backstop against a caller asking for the whole retention
// window in one response, not a paging window anyone should hit.
const maxReplayRange = 2000

// InsertReplayStep appends one recorded step. Ts must already be stamped.
func (s *Store) InsertReplayStep(st ReplayStep) error {
	_, err := s.db.Exec(
		`INSERT INTO replay_steps(account_id,device_id,ts,method,source,outcome,duration_ms,
		                          actor_label,frame_id,w,h,marker)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		st.AccountID, st.DeviceID, st.Ts, st.Method, st.Source, st.Outcome, st.DurationMs,
		st.ActorLabel, st.FrameID, st.W, st.H, st.Marker)
	return err
}

// ReplaySteps returns a device's steps between from and to (unix millis,
// inclusive), OLDEST FIRST — replay plays forward, unlike every other listing in
// this package. to <= 0 means "up to now"; limit <= 0 or over the cap uses
// maxReplayRange.
func (s *Store) ReplaySteps(deviceID string, from, to int64, limit int) ([]ReplayStep, error) {
	if to <= 0 {
		to = 1<<62 - 1
	}
	if limit <= 0 || limit > maxReplayRange {
		limit = maxReplayRange
	}
	rows, err := s.db.Query(
		`SELECT id,account_id,device_id,ts,method,source,outcome,duration_ms,actor_label,frame_id,w,h,marker
		   FROM replay_steps WHERE device_id=? AND ts>=? AND ts<=?
		  ORDER BY ts ASC, id ASC LIMIT ?`, deviceID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReplayStep
	for rows.Next() {
		var st ReplayStep
		if err := rows.Scan(&st.ID, &st.AccountID, &st.DeviceID, &st.Ts, &st.Method, &st.Source,
			&st.Outcome, &st.DurationMs, &st.ActorLabel, &st.FrameID, &st.W, &st.H, &st.Marker); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ReplayMark is the slim projection the session index is built from: enough to
// split a device's history into sessions and describe each one, without loading
// every row's full width.
type ReplayMark struct {
	Ts         int64
	ActorLabel string
	HasFrame   bool
}

// ReplayMarks returns up to limit of a device's most recent marks, OLDEST FIRST.
// The API groups them into sessions by time gap; see api.replaySessions.
func (s *Store) ReplayMarks(deviceID string, limit int) ([]ReplayMark, error) {
	if limit <= 0 || limit > maxReplayRange*10 {
		limit = maxReplayRange * 10
	}
	// Take the newest `limit` rows, then reverse in Go: SQLite has no way to
	// order a LIMIT window one way and return it the other.
	rows, err := s.db.Query(
		`SELECT ts,actor_label,frame_id<>'' FROM replay_steps
		  WHERE device_id=? ORDER BY ts DESC, id DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var desc []ReplayMark
	for rows.Next() {
		var m ReplayMark
		if err := rows.Scan(&m.Ts, &m.ActorLabel, &m.HasFrame); err != nil {
			return nil, err
		}
		desc = append(desc, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]ReplayMark, len(desc))
	for i, m := range desc {
		out[len(desc)-1-i] = m
	}
	return out, nil
}

// ReplayFrame resolves a frame id to the device and account that own it, so a
// frame request can be authorized without trusting the id itself. Returns
// ErrNotFound when no step references it.
func (s *Store) ReplayFrame(frameID string) (deviceID, accountID string, err error) {
	err = s.db.QueryRow(
		`SELECT device_id,account_id FROM replay_steps WHERE frame_id=? LIMIT 1`, frameID).
		Scan(&deviceID, &accountID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	return deviceID, accountID, nil
}

// replayPruneBatch is how many rows one delete statement removes. Same reasoning
// as pruneBatch in activities.go: store.Open pins the pool to a single
// connection, so an unbounded DELETE over a table that exists to accumulate
// stalls every unrelated request behind it for as long as it runs. Batching
// doesn't make the sweep faster — it bounds how long any one request can be
// stuck waiting on it.
//
// A var, not a const, only so tests can lower it and exercise the multi-batch
// path without inserting thousands of rows.
var replayPruneBatch = 500

// PruneReplayStepsBefore deletes every step older than beforeTs (unix millis)
// and returns the frame ids it removed, so the caller can delete those files.
// On an error partway through, the ids collected so far are still returned: the
// rows are gone whether or not the sweep finished, and their files must follow.
func (s *Store) PruneReplayStepsBefore(beforeTs int64) ([]string, error) {
	var frames []string
	for {
		ids, batchFrames, err := s.replayBatch(
			`SELECT id,frame_id FROM replay_steps WHERE ts<? ORDER BY id ASC LIMIT ?`,
			beforeTs, replayPruneBatch)
		if err != nil {
			return frames, err
		}
		if len(ids) == 0 {
			return frames, nil
		}
		if err := s.deleteReplayRows(ids); err != nil {
			return frames, err
		}
		frames = append(frames, batchFrames...)
		if len(ids) < replayPruneBatch {
			return frames, nil
		}
	}
}

// TrimReplayDevice keeps only the newest `keep` steps for a device and returns
// the frame ids it removed. This is the disk backstop the age window can't
// provide: one busy agent can record thousands of frames inside the retention
// window. keep <= 0 disables the trim.
func (s *Store) TrimReplayDevice(deviceID string, keep int) ([]string, error) {
	if keep <= 0 {
		return nil, nil
	}
	var frames []string
	for {
		// LIMIT -1 OFFSET keep is SQLite's "everything past the first N rows".
		ids, batchFrames, err := s.replayBatch(
			`SELECT id,frame_id FROM (
			   SELECT id,frame_id FROM replay_steps WHERE device_id=?
			    ORDER BY ts DESC, id DESC LIMIT -1 OFFSET ?
			 ) ORDER BY id ASC LIMIT ?`,
			deviceID, keep, replayPruneBatch)
		if err != nil {
			return frames, err
		}
		if len(ids) == 0 {
			return frames, nil
		}
		if err := s.deleteReplayRows(ids); err != nil {
			return frames, err
		}
		frames = append(frames, batchFrames...)
		if len(ids) < replayPruneBatch {
			return frames, nil
		}
	}
}

// DeleteReplayDevice removes every step for a device and returns the frame ids,
// for when the device itself is deleted.
func (s *Store) DeleteReplayDevice(deviceID string) ([]string, error) {
	var frames []string
	for {
		ids, batchFrames, err := s.replayBatch(
			`SELECT id,frame_id FROM replay_steps WHERE device_id=? ORDER BY id ASC LIMIT ?`,
			deviceID, replayPruneBatch)
		if err != nil {
			return frames, err
		}
		if len(ids) == 0 {
			return frames, nil
		}
		if err := s.deleteReplayRows(ids); err != nil {
			return frames, err
		}
		frames = append(frames, batchFrames...)
		if len(ids) < replayPruneBatch {
			return frames, nil
		}
	}
}

// ReplayDeviceIDs lists the devices that currently have any recorded steps, so
// the per-device trim only visits devices that need it.
func (s *Store) ReplayDeviceIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT device_id FROM replay_steps`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// replayBatch runs a select that yields (id, frame_id) pairs and splits them
// into row ids to delete and the non-empty frame ids among them.
func (s *Store) replayBatch(query string, args ...any) ([]int64, []string, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	var frames []string
	for rows.Next() {
		var id int64
		var frame string
		if err := rows.Scan(&id, &frame); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		if frame != "" {
			frames = append(frames, frame)
		}
	}
	return ids, frames, rows.Err()
}

// deleteReplayRows removes one batch by primary key. The ids come from
// replayBatch, never from a caller, so they are interpolated into the IN list
// directly — database/sql has no way to bind a variadic list.
func (s *Store) deleteReplayRows(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	args := make([]any, len(ids))
	placeholders := make([]byte, 0, len(ids)*2)
	for i, id := range ids {
		args[i] = id
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
	}
	_, err := s.db.Exec(`DELETE FROM replay_steps WHERE id IN (`+string(placeholders)+`)`, args...)
	return err
}
