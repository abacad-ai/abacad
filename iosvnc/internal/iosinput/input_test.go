package iosinput

import (
	"testing"
	"time"

	"github.com/danielpaulus/go-ios/ios/hid"
)

type fakeDigitizer struct {
	events []string
	points []hid.NormalizedPoint
}

func (f *fakeDigitizer) TouchDown(p hid.NormalizedPoint, _ hid.Edge) error {
	f.events = append(f.events, "down")
	f.points = append(f.points, p)
	return nil
}

func (f *fakeDigitizer) TouchMove(p hid.NormalizedPoint) error {
	f.events = append(f.events, "move")
	f.points = append(f.points, p)
	return nil
}

func (f *fakeDigitizer) TouchUp(p hid.NormalizedPoint) error {
	f.events = append(f.events, "up")
	f.points = append(f.points, p)
	return nil
}

func (f *fakeDigitizer) Close() error { return nil }

type fakeControls struct {
	presses []hid.Button
}

func (f *fakeControls) PressButton(b hid.Button, _ time.Duration) error {
	f.presses = append(f.presses, b)
	return nil
}

func (f *fakeControls) Close() error { return nil }

func TestPointerMapsPressDragRelease(t *testing.T) {
	dig, ctrl := &fakeDigitizer{}, &fakeControls{}
	touch := newTouch(dig, ctrl, 100, 200)

	if err := touch.Pointer(1, 50, 100); err != nil { // press at the centre
		t.Fatal(err)
	}
	if err := touch.Pointer(1, 75, 150); err != nil { // drag
		t.Fatal(err)
	}
	if err := touch.Pointer(0, 75, 150); err != nil { // release
		t.Fatal(err)
	}

	want := []string{"down", "move", "up"}
	if len(dig.events) != len(want) {
		t.Fatalf("events = %v, want %v", dig.events, want)
	}
	for i := range want {
		if dig.events[i] != want[i] {
			t.Fatalf("events = %v, want %v", dig.events, want)
		}
	}
	if got := dig.points[0]; got.X != 0.5 || got.Y != 0.5 {
		t.Fatalf("press point = %+v, want 0.5/0.5", got)
	}
	if got := dig.points[1]; got.X != 0.75 || got.Y != 0.75 {
		t.Fatalf("drag point = %+v, want 0.75/0.75", got)
	}
	if len(ctrl.presses) != 0 {
		t.Fatalf("left button pressed Home: %v", ctrl.presses)
	}
}

// TestRightButtonHomesOnce is the behaviour a viewer relies on: holding the
// right button must not machine-gun the Home button.
func TestRightButtonHomesOnce(t *testing.T) {
	dig, ctrl := &fakeDigitizer{}, &fakeControls{}
	touch := newTouch(dig, ctrl, 100, 200)

	for i := 0; i < 4; i++ {
		if err := touch.Pointer(2, 10, 10); err != nil {
			t.Fatal(err)
		}
	}
	if len(ctrl.presses) != 1 {
		t.Fatalf("Home presses = %d, want 1", len(ctrl.presses))
	}
	if len(dig.events) != 0 {
		t.Fatalf("right button produced touches: %v", dig.events)
	}
}

func TestPointerClampsOutOfRange(t *testing.T) {
	dig, ctrl := &fakeDigitizer{}, &fakeControls{}
	touch := newTouch(dig, ctrl, 100, 200)

	if err := touch.Pointer(1, 1000, 5000); err != nil {
		t.Fatal(err)
	}
	if got := dig.points[0]; got.X != 1 || got.Y != 1 {
		t.Fatalf("point = %+v, want clamped to 1/1", got)
	}
	if err := touch.Pointer(0, -40, -90); err != nil {
		t.Fatal(err)
	}
	if got := dig.points[1]; got.X != 0 || got.Y != 0 {
		t.Fatalf("point = %+v, want clamped to 0/0", got)
	}
}

func TestEscapePressesHome(t *testing.T) {
	dig, ctrl := &fakeDigitizer{}, &fakeControls{}
	touch := newTouch(dig, ctrl, 100, 200)

	if err := touch.Key(true, 0xff1b); err != nil { // Escape down
		t.Fatal(err)
	}
	if err := touch.Key(false, 0xff1b); err != nil { // Escape up
		t.Fatal(err)
	}
	if len(ctrl.presses) != 1 {
		t.Fatalf("Home presses = %d, want 1", len(ctrl.presses))
	}
}
