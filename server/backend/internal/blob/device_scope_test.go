package blob

import (
	"strings"
	"testing"

	"abacad/internal/store"
)

// A blob that came off a device must not be readable by every identity in the
// account. Before blobs recorded their origin, Service.Open compared account_id
// and nothing else and the authorizer discarded the API key's scope entirely —
// so a key restricted to one device could download a file pulled from another,
// and any device token could read its siblings' uploads. That undid per-device
// gating of pull_file at the exact moment the bytes moved.
func TestCallerCanReach(t *testing.T) {
	scopeA := store.KeyScope{DeviceIDs: []string{"dev_a"}}
	wildcard := store.KeyScope{AllDevices: true}

	cases := []struct {
		name     string
		caller   Caller
		blobFrom string
		want     bool
	}{
		{"owner session reaches any device", Caller{AccountID: "acc"}, "dev_a", true},
		{"key scoped to A reaches A", Caller{AccountID: "acc", Scope: &scopeA}, "dev_a", true},
		{"key scoped to A cannot reach B", Caller{AccountID: "acc", Scope: &scopeA}, "dev_b", false},
		{"wildcard key reaches any device", Caller{AccountID: "acc", Scope: &wildcard}, "dev_b", true},
		{"device reaches its own blob", Caller{AccountID: "acc", DeviceID: "dev_a"}, "dev_a", true},
		{"device cannot reach a sibling's blob", Caller{AccountID: "acc", DeviceID: "dev_a"}, "dev_b", false},
		// An agent staging bytes for send_file leaves no origin device, so
		// nothing device-specific restricts reading it back.
		{"scoped key reaches a device-less blob", Caller{AccountID: "acc", Scope: &scopeA}, "", true},
		{"device reaches a device-less blob", Caller{AccountID: "acc", DeviceID: "dev_a"}, "", true},
	}

	for _, tc := range cases {
		if got := tc.caller.CanReach(tc.blobFrom); got != tc.want {
			t.Errorf("%s: CanReach(%q) = %v, want %v", tc.name, tc.blobFrom, got, tc.want)
		}
	}
}

// The origin device has to survive the round trip, or CanReach has nothing to
// judge and every blob silently becomes unrestricted.
func TestPutRecordsOriginDevice(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	acc, err := st.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	svc := &Service{Store: st, Dir: t.TempDir(), MaxBytes: 1 << 20}

	b, err := svc.Put(acc.ID, "dev_a", "text/plain", strings.NewReader("from device a"))
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := st.BlobByID(b.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.DeviceID != "dev_a" {
		t.Fatalf("origin device = %q, want dev_a", got.DeviceID)
	}
}
