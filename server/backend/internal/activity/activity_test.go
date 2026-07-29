package activity

import (
	"testing"
	"time"

	"abacad/internal/store"
)

type stubLocator struct {
	calls []string
	out   map[string][2]string
}

func (s *stubLocator) Lookup(ip string) (string, string) {
	s.calls = append(s.calls, ip)
	if v, ok := s.out[ip]; ok {
		return v[0], v[1]
	}
	return "", ""
}

func openTemp(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func waitRows(t *testing.T, st *store.Store, account string, n int) []store.Activity {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := st.Activities(account, store.ActivityFilter{})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d row(s), want %d", len(got), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Enrichment happens in the write loop, so no caller has to know geo exists. This
// pins that: a plain Record with only an IP comes back out with a location.
func TestRecordEnrichesLocation(t *testing.T) {
	st := openTemp(t)
	geo := &stubLocator{out: map[string][2]string{"81.2.69.142": {"GB", "London"}}}
	r := New(st, 0, WithLocator(geo))

	r.Record(store.Activity{AccountID: "acc1", Kind: "auth.login", IP: "81.2.69.142"})
	rows := waitRows(t, st, "acc1", 1)
	if rows[0].Country != "GB" || rows[0].City != "London" {
		t.Errorf("got country=%q city=%q, want GB/London", rows[0].Country, rows[0].City)
	}
}

// A row with no IP must not reach the locator at all — SSH and internal events
// can lack one, and looking up "" is a wasted call per row.
func TestRecordSkipsLookupWithoutIP(t *testing.T) {
	st := openTemp(t)
	geo := &stubLocator{}
	r := New(st, 0, WithLocator(geo))

	r.Record(store.Activity{AccountID: "acc1", Kind: "auth.logout"})
	waitRows(t, st, "acc1", 1)
	if len(geo.calls) != 0 {
		t.Errorf("locator consulted for a row with no IP: %v", geo.calls)
	}
}

// Geo is optional: with no locator the trail still records, just without a
// location. This is the default configuration, so it must not be the untested one.
func TestRecordWithoutLocator(t *testing.T) {
	st := openTemp(t)
	r := New(st, 0)

	r.Record(store.Activity{AccountID: "acc1", Kind: "auth.login", IP: "81.2.69.142"})
	rows := waitRows(t, st, "acc1", 1)
	if rows[0].IP != "81.2.69.142" {
		t.Errorf("IP = %q", rows[0].IP)
	}
	if rows[0].Country != "" || rows[0].City != "" {
		t.Errorf("want no location without a locator, got %q/%q", rows[0].Country, rows[0].City)
	}
}

// An address the database doesn't know stays blank rather than acquiring a guess.
func TestRecordUnknownAddressStaysBlank(t *testing.T) {
	st := openTemp(t)
	r := New(st, 0, WithLocator(&stubLocator{}))

	r.Record(store.Activity{AccountID: "acc1", Kind: "auth.login", IP: "192.0.2.1"})
	rows := waitRows(t, st, "acc1", 1)
	if rows[0].Country != "" || rows[0].City != "" {
		t.Errorf("want blank location, got %q/%q", rows[0].Country, rows[0].City)
	}
}

// Dropped rows are counted so a gap in the trail is visible. Fill the buffer
// without draining it by never starting a write loop.
func TestDroppedCounter(t *testing.T) {
	r := &Recorder{ch: make(chan store.Activity, 2)} // no writeLoop: nothing drains
	for i := 0; i < 5; i++ {
		r.Record(store.Activity{AccountID: "acc1", Kind: "command"})
	}
	if got := r.Dropped(); got != 3 {
		t.Errorf("Dropped() = %d, want 3 (5 recorded, buffer of 2)", got)
	}
}

// A nil Recorder is a documented no-op so callers never guard.
func TestNilRecorder(t *testing.T) {
	var r *Recorder
	r.Record(store.Activity{Kind: "command"})
	if got := r.Dropped(); got != 0 {
		t.Errorf("Dropped() on nil = %d", got)
	}
}
