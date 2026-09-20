package uitree

import (
	"fmt"
	"testing"

	"abacad/internal/protocol"
)

func node(cls, text string, clickable bool, b [4]int) protocol.UITreeNode {
	return protocol.UITreeNode{Cls: cls, Text: text, Clickable: clickable, Bounds: b}
}

func treeOf(nodes ...protocol.UITreeNode) *protocol.UITree {
	return &protocol.UITree{Nodes: nodes}
}

func labels(ts []Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Label
	}
	return out
}

// TestExtractDropsZeroAreaMenuItems pins the rule that does most of the work on
// macOS. An AX walk covers the entire menu bar, and every item of a closed menu
// supports AXPress while reporting a zero-area frame — measured at 296 of
// Finder's 343 nodes. Selecting on the clickable flag alone would bury the real
// buttons under hundreds of things that cannot be clicked.
func TestExtractDropsZeroAreaMenuItems(t *testing.T) {
	nodes := []protocol.UITreeNode{node("AXButton", "OK", true, [4]int{100, 200, 180, 230})}
	for i := 0; i < 50; i++ {
		nodes = append(nodes, node("AXMenuItem", fmt.Sprintf("item %d", i), true, [4]int{0, 0, 0, 0}))
	}

	got, st := Extract(treeOf(nodes...), 1920, 1080, MaxTargets)

	if len(got) != 1 || got[0].Label != "OK" {
		t.Fatalf("want exactly the OK button, got %v", labels(got))
	}
	if st.Nodes != 51 {
		t.Errorf("Stats.Nodes = %d, want 51", st.Nodes)
	}
	if st.Visible != 1 {
		t.Errorf("Stats.Visible = %d, want 1 (the 50 closed menu items are invisible)", st.Visible)
	}
}

// TestExtractBorrowsLabelFromContainedText pins the rule Android depends on.
// Android hangs the click handler on a layout container while the words live in
// a child TextView, so the clickable node carries no text of its own. Without
// the geometric merge the target comes back anonymous and no caller can name it.
func TestExtractBorrowsLabelFromContainedText(t *testing.T) {
	got, _ := Extract(treeOf(
		node("android.widget.LinearLayout", "", true, [4]int{0, 100, 1440, 300}),
		node("android.widget.TextView", "Connected to the home network", false, [4]int{40, 230, 900, 280}),
		node("android.widget.TextView", "Wi-Fi", false, [4]int{40, 150, 400, 220}),
	), 1440, 3040, MaxTargets)

	if len(got) != 1 {
		t.Fatalf("want 1 target, got %v", labels(got))
	}
	// Most specific wins: the row holds both a title and a subtitle, and the
	// tighter box is the one that names the control.
	if got[0].Label != "Wi-Fi" {
		t.Errorf("Label = %q, want %q (the smallest contained text)", got[0].Label, "Wi-Fi")
	}
}

// TestExtractBorrowsRowLabelFromSibling covers the shape containment alone
// misses. On a settings row the switch is the clickable thing and its words sit
// at the far end of the row, entirely outside its bounds. Found by running the
// extractor over a real captured Compose screen, where containment alone left
// 4 of 5 controls anonymous.
func TestExtractBorrowsRowLabelFromSibling(t *testing.T) {
	got, _ := Extract(treeOf(
		node("Text", "See the screen", false, [4]int{60, 400, 700, 460}),
		node("Switch", "", true, [4]int{1200, 400, 1340, 460}),
	), 1440, 3040, MaxTargets)

	if len(got) != 1 {
		t.Fatalf("want 1 target, got %d", len(got))
	}
	if got[0].Label != "See the screen" {
		t.Errorf("Label = %q, want %q", got[0].Label, "See the screen")
	}
}

// TestExtractDoesNotPoachAnotherControlsLabel pins the ownership rule that keeps
// the sideways borrow honest. Text inside a control belongs to it; a neighbour
// with no words of its own stays anonymous rather than borrowing them, because
// two targets sharing a name is worse than one having none.
func TestExtractDoesNotPoachAnotherControlsLabel(t *testing.T) {
	got, _ := Extract(treeOf(
		node("Button", "", true, [4]int{0, 100, 200, 160}),
		node("Text", "Cancel", false, [4]int{20, 110, 180, 150}), // inside the left button
		node("Button", "", true, [4]int{300, 100, 500, 160}),     // same row, no words
	), 800, 600, MaxTargets)

	if len(got) != 2 {
		t.Fatalf("want 2 targets, got %d: %v", len(got), labels(got))
	}
	byX := map[int]string{}
	for _, g := range got {
		byX[g.X] = g.Label
	}
	if byX[100] != "Cancel" {
		t.Errorf("left button label = %q, want %q", byX[100], "Cancel")
	}
	if byX[400] != "" {
		t.Errorf("right button label = %q, want empty — it must not poach", byX[400])
	}
}

// TestExtractRowLabelStaysOnItsLine guards the sideways borrow from reaching
// across the screen: text on another line is not this control's label.
func TestExtractRowLabelStaysOnItsLine(t *testing.T) {
	got, _ := Extract(treeOf(
		node("Text", "Above", false, [4]int{60, 100, 400, 150}),
		node("Switch", "", true, [4]int{1200, 400, 1340, 460}),
	), 1440, 3040, MaxTargets)

	if len(got) != 1 {
		t.Fatalf("want 1 target, got %d", len(got))
	}
	if got[0].Label != "" {
		t.Errorf("Label = %q, want empty — %q is on a different line", got[0].Label, "Above")
	}
}

// TestExtractDedupesSameControlUnderSeveralRoles covers both shapes of duplicate
// the trees actually emit: one element surfacing under two roles, and a wrapper
// drawn tightly around the control it wraps. Both resolve to the same click
// point, so both are the same target. Measured at 16 clickable -> 10 distinct on
// a Finder window.
func TestExtractDedupesSameControlUnderSeveralRoles(t *testing.T) {
	got, _ := Extract(treeOf(
		node("AXButton", "Save", true, [4]int{10, 10, 110, 50}),
		node("AXStaticText", "Save", true, [4]int{10, 10, 110, 50}), // same box, other role
		node("AXGroup", "Save", true, [4]int{0, 0, 120, 60}),        // wrapper, same center
	), 800, 600, MaxTargets)

	if len(got) != 1 {
		t.Fatalf("want 1 deduped target, got %d: %v", len(got), labels(got))
	}
}

// TestExtractDropsOffScreenNodes covers the other half of invisibility. A list
// keeps rows realized past the viewport and a drawer keeps its contents laid out
// while closed; those nodes carry plausible bounds and a clickable flag, but
// they are not on screen and acting on them does nothing.
func TestExtractDropsOffScreenNodes(t *testing.T) {
	got, _ := Extract(treeOf(
		node("Button", "visible", true, [4]int{0, 100, 200, 200}),
		node("Button", "below", true, [4]int{0, 4000, 200, 4100}),
		node("Button", "right", true, [4]int{2000, 100, 2200, 200}),
		node("Button", "above", true, [4]int{0, -300, 200, -100}),
	), 1440, 3040, MaxTargets)

	if len(got) != 1 || got[0].Label != "visible" {
		t.Fatalf("want only the on-screen target, got %v", labels(got))
	}
}

// TestExtractOrdersByReadingOrder pins index assignment. Indices are handed out
// after sorting so they run the way a person scans the screen — which also means
// a truncated list loses the bottom of the screen rather than an arbitrary slice.
func TestExtractOrdersByReadingOrder(t *testing.T) {
	got, _ := Extract(treeOf(
		node("Button", "third", true, [4]int{0, 500, 100, 560}),
		node("Button", "second", true, [4]int{300, 100, 400, 160}),
		node("Button", "first", true, [4]int{0, 100, 100, 160}),
	), 800, 600, MaxTargets)

	want := []string{"first", "second", "third"}
	gotLabels := labels(got)
	if len(gotLabels) != len(want) {
		t.Fatalf("got %v, want %v", gotLabels, want)
	}
	for i := range want {
		if gotLabels[i] != want[i] {
			t.Errorf("position %d = %q, want %q", i, gotLabels[i], want[i])
		}
		if got[i].Index != i {
			t.Errorf("target %q has Index %d, want %d", got[i].Label, got[i].Index, i)
		}
	}
}

// TestExtractKeepsAnonymousTargets guards against over-filtering. A scrollbar or
// an icon-only button has no text and nothing inside it to borrow, but it is
// still a real control. Measured at 1 of 6 targets (a macOS dialog) and 3 of 24
// (an Android screen), so dropping the nameless ones would lose real targets.
func TestExtractKeepsAnonymousTargets(t *testing.T) {
	got, st := Extract(treeOf(
		node("AXButton", "", true, [4]int{1670, 481, 1676, 931}),
	), 1920, 1080, MaxTargets)

	if len(got) != 1 || got[0].Label != "" {
		t.Fatalf("want one anonymous target, got %v", labels(got))
	}
	if st.Clickable != 1 || st.Labelled != 0 {
		t.Errorf("Stats.Clickable/Labelled = %d/%d, want 1/0", st.Clickable, st.Labelled)
	}
}

// TestExtractTruncatesAndReports makes the cap visible. A caller that silently
// receives a subset cannot tell a short screen from a clipped one, so the count
// that was dropped is reported rather than swallowed.
func TestExtractTruncatesAndReports(t *testing.T) {
	var nodes []protocol.UITreeNode
	for i := 0; i < 10; i++ {
		nodes = append(nodes, node("Button", fmt.Sprintf("b%d", i), true, [4]int{0, i * 10, 50, i*10 + 5}))
	}

	got, st := Extract(treeOf(nodes...), 800, 600, 4)

	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if st.Truncated != 6 {
		t.Errorf("Stats.Truncated = %d, want 6", st.Truncated)
	}
	if got[0].Label != "b0" {
		t.Errorf("kept %q first, want the topmost target b0", got[0].Label)
	}
}

// TestExtractWithoutScreenSize covers a device that does not report its screen
// size: the off-screen checks are skipped rather than rejecting every node, so
// the zero-area and labelling passes still run.
func TestExtractWithoutScreenSize(t *testing.T) {
	got, _ := Extract(treeOf(
		node("AXMenuItem", "Copy", true, [4]int{0, 0, 0, 0}),
		node("AXButton", "OK", true, [4]int{10, 10, 50, 40}),
	), 0, 0, MaxTargets)

	if len(got) != 1 || got[0].Label != "OK" {
		t.Fatalf("want only OK, got %v", labels(got))
	}
}

// TestExtractNilTree covers the include_ui_tree=false path, where a screenshot
// result carries no tree at all.
func TestExtractNilTree(t *testing.T) {
	got, st := Extract(nil, 1920, 1080, MaxTargets)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
	if st.Nodes != 0 || st.Targets != 0 {
		t.Errorf("Stats = %+v, want zero", st)
	}
}

// TestExtractCenterPoint pins the coordinate contract: callers act on the center
// of a control, not the corner of its box, because a bounding box's top-left is
// not reliably inside the thing it describes.
func TestExtractCenterPoint(t *testing.T) {
	got, _ := Extract(treeOf(
		node("AXButton", "OK", true, [4]int{1470, 945, 1570, 971}),
	), 1920, 1080, MaxTargets)

	if len(got) != 1 {
		t.Fatalf("want 1 target, got %d", len(got))
	}
	if got[0].X != 1520 || got[0].Y != 958 {
		t.Errorf("center = (%d, %d), want (1520, 958)", got[0].X, got[0].Y)
	}
}
