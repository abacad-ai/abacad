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
	"sync/atomic"
	"time"

	"abacad/internal/store"
)

// dropReportInterval bounds how often a full buffer is reported. Drops arrive in
// bursts (that is what a full buffer means), so this coalesces them into one line
// per minute rather than one per lost row.
const dropReportInterval = time.Minute

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
	KindConsent      = "consent.attested"  // operator attested authorization (enrollment / humanize opt-in)
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

// Locator resolves a client IP to a coarse location. Satisfied by
// *geoip.Locator; kept as an interface here so the trail does not depend on a
// geo database being present, in tests or in production.
type Locator interface {
	Lookup(ip string) (country, city string)
}

// Recorder is the async writer. Safe for concurrent use; a nil *Recorder is a
// no-op so callers never need to guard.
type Recorder struct {
	st      *store.Store
	ch      chan store.Activity
	geo     Locator       // nil when no geo database is configured
	dropped atomic.Uint64 // rows lost to a full buffer; see reportDrops
}

// Option configures a Recorder at construction, before its goroutines start.
type Option func(*Recorder)

// WithLocator enriches each row with the country and city its IP resolves to.
// Without it, those columns stay empty and everything else is unchanged.
func WithLocator(l Locator) Option {
	return func(r *Recorder) { r.geo = l }
}

// New starts a Recorder writing to st and pruning rows older than retention
// (<= 0 disables pruning) once at start and then every 6 hours. Options are
// applied before any goroutine starts, so they are not racing the write loop.
func New(st *store.Store, retention time.Duration, opts ...Option) *Recorder {
	r := &Recorder{st: st, ch: make(chan store.Activity, 1024)}
	for _, opt := range opts {
		opt(r)
	}
	go r.writeLoop()
	go r.reportDrops()
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
	default:
		// Full — drop rather than stall the caller, but count it. A silent gap in
		// the trail is indistinguishable from "nothing happened", which is the
		// wrong answer to give someone auditing their account: the buffer fills
		// under exactly the sustained load or write-lock stall an incident
		// produces. reportDrops surfaces the count.
		r.dropped.Add(1)
	}
}

// Dropped is the number of rows lost to a full buffer since start. Non-zero means
// the trail has holes.
func (r *Recorder) Dropped() uint64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// reportDrops logs the running drop count whenever it grows, so a gap shows up in
// the server log even though the affected rows never reached the database.
func (r *Recorder) reportDrops() {
	var last uint64
	for {
		time.Sleep(dropReportInterval)
		if n := r.dropped.Load(); n > last {
			log.Printf("[activity] dropped %d row(s) — trail buffer full, history has gaps (total %d)", n-last, n)
			last = n
		}
	}
}

func (r *Recorder) writeLoop() {
	for a := range r.ch {
		// Geo is resolved here rather than at the ~25 places that build a row: this
		// goroutine is already off every caller's hot path, and the lookup is a
		// memory-mapped read that costs a fraction of the SQLite write it precedes.
		// A row that already carries a country keeps it (nothing sets one today,
		// but an explicit value should never be silently overwritten).
		if r.geo != nil && a.IP != "" && a.Country == "" {
			a.Country, a.City = r.geo.Lookup(a.IP)
		}
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
