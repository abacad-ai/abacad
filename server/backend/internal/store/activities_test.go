package store

import "testing"

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
