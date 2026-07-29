package store

import (
	"testing"
	"unicode/utf8"
)

func TestActivities(t *testing.T) {
	s := openTemp(t)
	ins := func(a Activity) {
		t.Helper()
		if err := s.InsertActivity(a); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	ins(Activity{AccountID: "acc1", Ts: 1000, Kind: "auth.login", Source: "dashboard"})
	ins(Activity{AccountID: "acc1", DeviceID: "dev1", Ts: 2000, Kind: "device.connected"})
	ins(Activity{AccountID: "acc1", DeviceID: "dev1", Ts: 3000, Kind: "command", Method: "tap", Source: "agent", Outcome: "ok", DurationMs: 42})
	ins(Activity{AccountID: "acc1", DeviceID: "dev2", Ts: 4000, Kind: "command", Method: "screenshot", Source: "dashboard", Outcome: "ok"})
	ins(Activity{AccountID: "acc2", Ts: 5000, Kind: "auth.login"}) // another account

	all, err := s.Activities("acc1", ActivityFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("want 4 rows, got %d", len(all))
	}
	if all[0].Kind != "command" || all[0].Method != "screenshot" {
		t.Fatalf("want newest first, got %+v", all[0])
	}

	// Category-prefix kind filter: "device" matches device.connected only.
	byKind, err := s.Activities("acc1", ActivityFilter{Kind: "device"})
	if err != nil || len(byKind) != 1 || byKind[0].Kind != "device.connected" {
		t.Fatalf("kind filter: %v %+v", err, byKind)
	}
	// Exact kind still works.
	if got, _ := s.Activities("acc1", ActivityFilter{Kind: "command"}); len(got) != 2 {
		t.Fatalf("want 2 commands, got %d", len(got))
	}
	if got, _ := s.Activities("acc1", ActivityFilter{DeviceID: "dev1"}); len(got) != 2 {
		t.Fatalf("device filter: got %d", len(got))
	}
	if got, _ := s.Activities("acc1", ActivityFilter{Source: "agent"}); len(got) != 1 || got[0].Method != "tap" {
		t.Fatalf("source filter: %+v", got)
	}

	// Keyset pagination: page of 2, then everything before the last id.
	page1, _ := s.Activities("acc1", ActivityFilter{Limit: 2})
	if len(page1) != 2 {
		t.Fatalf("page1: got %d", len(page1))
	}
	page2, _ := s.Activities("acc1", ActivityFilter{Limit: 2, BeforeID: page1[1].ID})
	if len(page2) != 2 || page2[0].ID >= page1[1].ID {
		t.Fatalf("page2: %+v", page2)
	}

	// Prune drops rows older than the cutoff, across accounts.
	n, err := s.PruneActivities(3000)
	if err != nil || n != 2 {
		t.Fatalf("prune: n=%d err=%v", n, err)
	}
	if got, _ := s.Activities("acc1", ActivityFilter{}); len(got) != 2 {
		t.Fatalf("after prune: got %d", len(got))
	}
}

// Provenance round-trips through insert, select and both new filters. The columns
// arrive via ADD COLUMN migrations, so this also proves the migration applied.
func TestActivityProvenance(t *testing.T) {
	s := openTemp(t)
	ins := func(a Activity) {
		t.Helper()
		if err := s.InsertActivity(a); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	ins(Activity{
		AccountID: "acc1", DeviceID: "dev1", Ts: 1000, Kind: "command", Method: "tap", Source: "agent",
		ActorKind: ActorAPIKey, ActorID: "apikey_aaa", ActorLabel: "laptop agent",
		IP: "203.0.113.9", UserAgent: "abacad-cli/1.0",
	})
	ins(Activity{
		AccountID: "acc1", DeviceID: "dev1", Ts: 2000, Kind: "command", Method: "tap", Source: "agent",
		ActorKind: ActorAPIKey, ActorID: "apikey_bbb", ActorLabel: "ci runner",
		IP: "198.51.100.4",
	})
	// No actor at all: a failed sign-in is an unknown party, and must stay that way
	// rather than inheriting anyone's identity.
	ins(Activity{AccountID: "acc1", Ts: 3000, Kind: "auth.login_failed", IP: "203.0.113.9"})

	all, err := s.Activities("acc1", ActivityFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 rows, got %d", len(all))
	}
	if all[0].ActorKind != "" || all[0].ActorID != "" || all[0].ActorLabel != "" {
		t.Errorf("failed sign-in should carry no actor, got %+v", all[0])
	}
	if all[0].IP != "203.0.113.9" {
		t.Errorf("failed sign-in should still carry its IP, got %q", all[0].IP)
	}
	oldest := all[2]
	if oldest.ActorID != "apikey_aaa" || oldest.ActorLabel != "laptop agent" ||
		oldest.ActorKind != ActorAPIKey || oldest.UserAgent != "abacad-cli/1.0" {
		t.Errorf("actor did not round-trip: %+v", oldest)
	}

	// Filter by credential: one key's actions, not the other's.
	byActor, err := s.Activities("acc1", ActivityFilter{ActorID: "apikey_bbb"})
	if err != nil || len(byActor) != 1 || byActor[0].ActorLabel != "ci runner" {
		t.Fatalf("actor filter: %v %+v", err, byActor)
	}
	// Filter by address, spanning rows with and without an actor.
	byIP, _ := s.Activities("acc1", ActivityFilter{IP: "203.0.113.9"})
	if len(byIP) != 2 {
		t.Fatalf("ip filter: want 2, got %d", len(byIP))
	}
	// Both filters compose.
	both, _ := s.Activities("acc1", ActivityFilter{IP: "203.0.113.9", ActorID: "apikey_aaa"})
	if len(both) != 1 {
		t.Fatalf("actor+ip filter: want 1, got %d", len(both))
	}
}

// A hostile client controls its User-Agent, so the column is bounded — and the cut
// must not leave a half-written rune behind.
func TestActivityUserAgentTruncation(t *testing.T) {
	s := openTemp(t)
	// Multi-byte runes straddling the limit: a naive slice would split one.
	long := ""
	for len(long) < maxUserAgent*2 {
		long += "日本語"
	}
	if err := s.InsertActivity(Activity{AccountID: "acc1", Ts: 1, Kind: "auth.login", UserAgent: long}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := s.Activities("acc1", ActivityFilter{})
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v", err)
	}
	ua := got[0].UserAgent
	if len(ua) > maxUserAgent {
		t.Errorf("user agent not truncated: %d bytes", len(ua))
	}
	if !utf8.ValidString(ua) {
		t.Errorf("truncation left invalid UTF-8: %q", ua)
	}
}

// The migration runner re-runs every file on every boot and relies on the
// "duplicate column name" error being benign. Five ADD COLUMN migrations landed
// at once, so prove a second open still succeeds and that rows written by the
// first boot survive it — this is the upgrade path for every existing database.
func TestActivityColumnsSurviveReopen(t *testing.T) {
	dir := t.TempDir() + "/reopen.db"
	first, err := Open(dir)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := first.InsertActivity(Activity{
		AccountID: "acc1", Ts: 1000, Kind: "auth.login",
		ActorKind: ActorSession, ActorID: "sess_abc", ActorLabel: "user@x.test", IP: "203.0.113.9",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Second boot: every ADD COLUMN now errors as a duplicate and must be skipped.
	second, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen after migrations already applied: %v", err)
	}
	t.Cleanup(func() { second.Close() })

	got, err := second.Activities("acc1", ActivityFilter{})
	if err != nil {
		t.Fatalf("read after reopen: %v", err)
	}
	if len(got) != 1 || got[0].ActorID != "sess_abc" || got[0].IP != "203.0.113.9" {
		t.Fatalf("provenance lost across reopen: %+v", got)
	}
	// The index migration sorts after the columns it indexes and is genuinely
	// idempotent, so the actor filter must work on the reopened database too.
	if byActor, _ := second.Activities("acc1", ActivityFilter{ActorID: "sess_abc"}); len(byActor) != 1 {
		t.Fatalf("actor filter broken after reopen: %d rows", len(byActor))
	}
}

// Geo columns round-trip and filter, and arrive via the same ADD COLUMN path as
// the actor columns (0021-0022).
func TestActivityGeo(t *testing.T) {
	s := openTemp(t)
	ins := func(a Activity) {
		t.Helper()
		if err := s.InsertActivity(a); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	ins(Activity{AccountID: "acc1", Ts: 1000, Kind: "auth.login", IP: "81.2.69.142", Country: "GB", City: "London"})
	ins(Activity{AccountID: "acc1", Ts: 2000, Kind: "auth.login", IP: "89.160.20.112", Country: "SE", City: "Linköping"})
	// Country known, city not — a normal outcome, not an error.
	ins(Activity{AccountID: "acc1", Ts: 3000, Kind: "auth.login", IP: "203.0.113.9", Country: "US"})
	// Private address: no location at all.
	ins(Activity{AccountID: "acc1", Ts: 4000, Kind: "auth.login", IP: "10.0.0.1"})

	all, err := s.Activities("acc1", ActivityFilter{})
	if err != nil || len(all) != 4 {
		t.Fatalf("list: %v n=%d", err, len(all))
	}
	if all[3].Country != "GB" || all[3].City != "London" {
		t.Errorf("geo did not round-trip: %+v", all[3])
	}
	if all[2].City != "Linköping" {
		t.Errorf("non-ASCII city mangled: %q", all[2].City)
	}
	if all[1].Country != "US" || all[1].City != "" {
		t.Errorf("country-without-city should persist as-is: %+v", all[1])
	}
	if all[0].Country != "" || all[0].City != "" {
		t.Errorf("private address should carry no location: %+v", all[0])
	}

	byCountry, err := s.Activities("acc1", ActivityFilter{Country: "GB"})
	if err != nil || len(byCountry) != 1 || byCountry[0].City != "London" {
		t.Fatalf("country filter: %v %+v", err, byCountry)
	}
	// Composes with the other provenance filters.
	if got, _ := s.Activities("acc1", ActivityFilter{Country: "SE", IP: "89.160.20.112"}); len(got) != 1 {
		t.Fatalf("country+ip filter: got %d", len(got))
	}
}
