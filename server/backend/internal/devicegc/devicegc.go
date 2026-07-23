// Package devicegc enforces device-enrollment expiry on the hosted service.
//
// Enrollment expiry bounds the standing blast radius of a relay compromise: a
// device that isn't extended (or explicitly made permanent) stops being reachable
// after its TTL. Two things must happen at expiry, because a device authenticates
// only at connect time and an in-flight socket is never re-checked:
//
//   1. block reconnect — handled in the store by DeviceByTokenHash / DeviceByID
//      filtering out expired rows;
//   2. drop the live socket — handled here, by kicking the connection through the
//      hub on a periodic sweep.
//
// Rows are kept dormant after expiry (preserving the audit trail) and only hard-
// deleted after a grace window. The sweep mirrors activity.pruneLoop.
package devicegc

import (
	"log"
	"time"

	"abacad/internal/relay"
	"abacad/internal/store"
)

const sweepInterval = time.Hour

// Sweeper periodically kicks expired devices and reaps dormant rows past grace.
type Sweeper struct {
	st    *store.Store
	hub   *relay.Hub
	grace time.Duration // hard-delete this long after expiry; 0 = keep dormant forever
}

// Start launches the background sweep goroutine and returns the sweeper. The
// caller gates this on enrollment TTL being enabled (TTL > 0); a self-hosted
// instance with no TTL never starts it.
func Start(st *store.Store, hub *relay.Hub, grace time.Duration) *Sweeper {
	s := &Sweeper{st: st, hub: hub, grace: grace}
	go func() {
		for {
			s.sweep()
			time.Sleep(sweepInterval)
		}
	}()
	return s
}

// sweep runs one pass: kick expired live connections, then reap dormant rows.
func (s *Sweeper) sweep() {
	now := time.Now().Unix()

	// 1. Kick live connections of expired devices. Reconnect is already blocked by
	//    the store's expiry-aware lookups, so kicking is what actually ends access.
	if ids, err := s.st.ExpiredDeviceIDs(now); err != nil {
		log.Printf("[devicegc] list expired failed: %v", err)
	} else {
		for _, id := range ids {
			if s.hub.Kick(id) {
				log.Printf("[devicegc] kicked expired device %s", id)
			}
		}
	}

	// 2. Hard-delete devices that have been expired longer than the grace window.
	if s.grace > 0 {
		cutoff := time.Now().Add(-s.grace).Unix()
		if n, err := s.st.DeleteDevicesExpiredBefore(cutoff); err != nil {
			log.Printf("[devicegc] delete dormant failed: %v", err)
		} else if n > 0 {
			log.Printf("[devicegc] deleted %d devices dormant past grace", n)
		}
	}
}
