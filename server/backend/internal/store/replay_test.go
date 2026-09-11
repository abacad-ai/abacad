package store

import "testing"

func TestReplayStepsRangeAndOrder(t *testing.T) {
	s := openTemp(t)
	ins := func(st ReplayStep) {
		t.Helper()
		if err := s.InsertReplayStep(st); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	ins(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 1000, Method: "screenshot", Outcome: "ok", FrameID: "frm_a", W: 1080, H: 2400})
	ins(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 2000, Method: "tap", Outcome: "ok", Marker: `{"kind":"tap","x":10,"y":20}`})
	ins(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 3000, Method: "screenshot", Outcome: "ok", FrameID: "frm_b"})
	ins(ReplayStep{AccountID: "acc1", DeviceID: "dev2", Ts: 2500, Method: "screenshot", Outcome: "ok", FrameID: "frm_c"})

	// Oldest first — replay plays forward, unlike every other listing here.
	all, err := s.ReplaySteps("dev1", 0, 0, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 steps for dev1, got %d", len(all))
	}
	if all[0].Ts != 1000 || all[2].Ts != 3000 {
		t.Fatalf("want oldest first, got %d..%d", all[0].Ts, all[2].Ts)
	}
	if all[0].FrameID != "frm_a" || all[0].W != 1080 || all[0].H != 2400 {
		t.Fatalf("frame columns lost: %+v", all[0])
	}
	if all[1].Marker != `{"kind":"tap","x":10,"y":20}` {
		t.Fatalf("marker lost: %q", all[1].Marker)
	}

	// A range is inclusive at both ends and scoped to one device.
	mid, _ := s.ReplaySteps("dev1", 2000, 3000, 0)
	if len(mid) != 2 || mid[0].Ts != 2000 {
		t.Fatalf("range: %+v", mid)
	}
	if got, _ := s.ReplaySteps("dev2", 0, 0, 0); len(got) != 1 {
		t.Fatalf("device scoping: got %d steps for dev2", len(got))
	}
}

func TestReplayMarksAndFrameLookup(t *testing.T) {
	s := openTemp(t)
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 1000, Method: "screenshot", FrameID: "frm_a", ActorLabel: "laptop agent"})
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 2000, Method: "tap"})

	marks, err := s.ReplayMarks("dev1", 0)
	if err != nil {
		t.Fatalf("marks: %v", err)
	}
	if len(marks) != 2 {
		t.Fatalf("want 2 marks, got %d", len(marks))
	}
	if marks[0].Ts != 1000 || !marks[0].HasFrame || marks[0].ActorLabel != "laptop agent" {
		t.Fatalf("first mark: %+v", marks[0])
	}
	if marks[1].HasFrame {
		t.Fatal("a frameless step reported HasFrame")
	}

	// A frame resolves to its owners, so the download route can authorize it
	// without trusting the id in the URL.
	dev, acc, err := s.ReplayFrame("frm_a")
	if err != nil || dev != "dev1" || acc != "acc1" {
		t.Fatalf("ReplayFrame = %q %q %v", dev, acc, err)
	}
	if _, _, err := s.ReplayFrame("frm_nope"); err != ErrNotFound {
		t.Fatalf("unknown frame err = %v, want ErrNotFound", err)
	}
}

// The prune must cross more than one batch — that is the whole reason it is
// batched (the pool is pinned to one connection, so an unbounded DELETE stalls
// every other request behind it). Shrink the batch rather than insert thousands.
func TestPruneReplayStepsBeforeBatches(t *testing.T) {
	s := openTemp(t)
	orig := replayPruneBatch
	replayPruneBatch = 3
	t.Cleanup(func() { replayPruneBatch = orig })

	for i := 0; i < 10; i++ {
		st := ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: int64(i * 100), Method: "screenshot"}
		if i%2 == 0 {
			st.FrameID = "frm_" + string(rune('a'+i))
		}
		if err := s.InsertReplayStep(st); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	// Rows at ts 0,100,...,400 go (5 rows, 3 of them with frames: 0, 200, 400).
	frames, err := s.PruneReplayStepsBefore(500)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("want 3 frame ids returned, got %d (%v)", len(frames), frames)
	}
	left, _ := s.ReplaySteps("dev1", 0, 0, 0)
	if len(left) != 5 || left[0].Ts != 500 {
		t.Fatalf("after prune: %d rows starting at %d", len(left), left[0].Ts)
	}
}

func TestTrimReplayDeviceKeepsNewest(t *testing.T) {
	s := openTemp(t)
	orig := replayPruneBatch
	replayPruneBatch = 2
	t.Cleanup(func() { replayPruneBatch = orig })

	for i := 0; i < 8; i++ {
		if err := s.InsertReplayStep(ReplayStep{
			AccountID: "acc1", DeviceID: "dev1", Ts: int64(i * 100),
			Method: "screenshot", FrameID: "frm_" + string(rune('a'+i)),
		}); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	// Another device must be untouched: the cap is per device.
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev2", Ts: 50, Method: "screenshot", FrameID: "frm_z"})

	frames, err := s.TrimReplayDevice("dev1", 3)
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("want 5 evicted frames, got %d (%v)", len(frames), frames)
	}
	left, _ := s.ReplaySteps("dev1", 0, 0, 0)
	if len(left) != 3 || left[0].Ts != 500 {
		t.Fatalf("after trim: %d rows starting at %d", len(left), left[0].Ts)
	}
	if got, _ := s.ReplaySteps("dev2", 0, 0, 0); len(got) != 1 {
		t.Fatalf("trim crossed device boundaries: dev2 has %d rows", len(got))
	}

	// keep <= 0 disables the trim rather than deleting everything — the knob's
	// documented "unlimited", and getting this backwards erases the feature.
	if frames, err := s.TrimReplayDevice("dev1", 0); err != nil || len(frames) != 0 {
		t.Fatalf("keep=0 removed %d frames (%v)", len(frames), err)
	}
	if got, _ := s.ReplaySteps("dev1", 0, 0, 0); len(got) != 3 {
		t.Fatalf("keep=0 changed the row count to %d", len(got))
	}
}

func TestDeleteReplayDeviceAndDeviceIDs(t *testing.T) {
	s := openTemp(t)
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 1, Method: "screenshot", FrameID: "frm_a"})
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev1", Ts: 2, Method: "tap"})
	_ = s.InsertReplayStep(ReplayStep{AccountID: "acc1", DeviceID: "dev2", Ts: 3, Method: "screenshot", FrameID: "frm_b"})

	ids, err := s.ReplayDeviceIDs()
	if err != nil || len(ids) != 2 {
		t.Fatalf("device ids = %v (%v)", ids, err)
	}

	frames, err := s.DeleteReplayDevice("dev1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(frames) != 1 || frames[0] != "frm_a" {
		t.Fatalf("want frm_a returned for file deletion, got %v", frames)
	}
	if got, _ := s.ReplaySteps("dev1", 0, 0, 0); len(got) != 0 {
		t.Fatalf("dev1 still has %d rows", len(got))
	}
	if got, _ := s.ReplaySteps("dev2", 0, 0, 0); len(got) != 1 {
		t.Fatal("deleting one device's replay took another's")
	}
}

// The replay flag is per device, defaults off for a device created before the
// column existed as well as one created after, and round-trips through every
// device read path.
func TestDeviceReplayFlag(t *testing.T) {
	s := openTemp(t)
	acc, err := s.CreateAccount("a@x.test", "hash")
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	d, _, err := s.CreateDevice(acc.ID, "phone", "android", 0)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	if d.Replay {
		t.Fatal("a new device must not be recording")
	}
	if err := s.SetDeviceReplay(d.ID, acc.ID, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ := s.DeviceByID(d.ID); !got.Replay {
		t.Fatal("DeviceByID lost the replay flag")
	}
	if got, _ := s.DeviceOwnedBy(d.ID, acc.ID); !got.Replay {
		t.Fatal("DeviceOwnedBy lost the replay flag")
	}
	list, _ := s.DevicesByAccount(acc.ID)
	if len(list) != 1 || !list[0].Replay {
		t.Fatal("DevicesByAccount lost the replay flag")
	}
	// Another account cannot flip it.
	if err := s.SetDeviceReplay(d.ID, "acc_other", false); err != ErrNotFound {
		t.Fatalf("cross-account set err = %v, want ErrNotFound", err)
	}
}
