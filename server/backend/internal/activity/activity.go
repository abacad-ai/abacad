// Package activity records the persistent, account-wide activity trail behind
// the dashboard's Activities page: sign-ins, credential changes, device
// lifecycle, SSH/tunnel access, and every relayed command.
//
// Writes are asynchronous: Record hands the row to a buffered channel and a
// single goroutine inserts it, so nothing on the command hot path ever waits on
// SQLite. If the buffer is full (the store is wedged), rows are dropped rather
// than blocking the relay — the trail is an observability surface, not a ledger.
// A background loop prunes rows past the retention window.
package activity

import (
	"log"
	"time"

	"abacad/internal/store"
)

// Kinds. Dotted "category.action"; the category is the dashboard's filter unit.
// KindCommand is bare: relayed commands carry method/source/outcome instead.
const (
	KindLogin        = "auth.login"
	KindLoginFailed  = "auth.login_failed"
	KindLogout       = "auth.logout"
	KindRegister     = "auth.register"
	KindDeviceCreate = "device.created"
	KindDeviceRename = "device.renamed"
	KindDeviceDelete = "device.deleted"
	KindDeviceToken  = "device.token_rotated"
	KindConnected    = "device.connected"
	KindDisconnected = "device.disconnected"
	KindMCPToken     = "mcp.token_rotated" // legacy; retained for old trail rows
	KindAPIKeyCreate = "apikey.created"
	KindAPIKeyUpdate = "apikey.updated"
	KindAPIKeyDelete = "apikey.deleted"
	KindSSHKeyAdd    = "ssh.key_added"
	KindSSHKeyRemove = "ssh.key_removed"
	KindSSHSession   = "ssh.session"
	KindTunnel       = "tunnel.opened"
	KindCommand      = "command"
)

// Recorder is the async writer. Safe for concurrent use; a nil *Recorder is a
// no-op so callers never need to guard.
type Recorder struct {
	st *store.Store
	ch chan store.Activity
}

// New starts a Recorder writing to st and pruning rows older than retention
// (<= 0 disables pruning) once at start and then every 6 hours.
func New(st *store.Store, retention time.Duration) *Recorder {
	r := &Recorder{st: st, ch: make(chan store.Activity, 1024)}
	go r.writeLoop()
	if retention > 0 {
		go r.pruneLoop(retention)
	}
	return r
}

// Record stamps and enqueues one activity row. Never blocks.
func (r *Recorder) Record(a store.Activity) {
	if r == nil {
		return
	}
	if a.Ts == 0 {
		a.Ts = time.Now().UnixMilli()
	}
	select {
	case r.ch <- a:
	default: // full — drop rather than stall the caller
	}
}

func (r *Recorder) writeLoop() {
	for a := range r.ch {
		if err := r.st.InsertActivity(a); err != nil {
			log.Printf("[activity] insert failed (kind=%s): %v", a.Kind, err)
		}
	}
}

func (r *Recorder) pruneLoop(retention time.Duration) {
	for {
		if n, err := r.st.PruneActivities(time.Now().Add(-retention).UnixMilli()); err != nil {
			log.Printf("[activity] prune failed: %v", err)
		} else if n > 0 {
			log.Printf("[activity] pruned %d rows past retention", n)
		}
		time.Sleep(6 * time.Hour)
	}
}
