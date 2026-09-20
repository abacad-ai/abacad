// Package uitree turns a device's raw accessibility tree into the small set of
// things an agent can actually act on.
//
// The raw tree is not that set, and the gap is much larger than it looks.
// Measured against real devices on 2026-09-20:
//
//	macOS Finder     343 nodes, 312 flagged clickable ->  10 real targets
//	macOS dialog     214 nodes, 187 flagged clickable ->   6 real targets
//	Android search   174 nodes,  85 flagged clickable ->  24 real targets
//	Android launcher  41 nodes,  33 flagged clickable ->  19 real targets
//
// So the clickable flag alone over-selects by ~30x on macOS. The cause is
// concrete: a macOS tree walks the whole menu bar, and every closed menu item
// supports AXPress while having zero-area bounds — 296 of Finder's 343 nodes are
// AXMenuItem. Filtering on the flag would hand a caller 312 candidates of which
// 302 cannot be clicked at all.
//
// Extract applies three passes, in this order:
//
//  1. Visibility — drop zero-area and off-screen nodes. This is the single
//     highest-value rule; on macOS it removes ~90% of the tree by itself.
//  2. Selection and labelling — take every clickable node, plus content that
//     names itself but is not flagged clickable (macOS reports a desktop icon
//     as an AXImage with no AXPress, so the flag alone loses every icon on the
//     desktop). A clickable node with no text of its own adopts nearby text,
//     first from inside its own bounds and then from alongside it. Android needs
//     both: the click handler usually sits on a LinearLayout container while the
//     words live in a child TextView (measured 79 LinearLayout vs 81 TextView on
//     one screen), and on a settings row the control is a switch at one end with
//     its label a sibling at the other — containment alone left 4 of 5 controls
//     anonymous on a real Compose screen. Text already borrowed as a label is
//     not offered again on its own, and neither are captions or containers.
//  3. Deduplication — collapse targets sharing a label and a click point, which
//     the tree emits whenever one visual element appears under several roles.
//
// Targets carry a center point instead of bounds. The top-left corner of a
// bounding box is not reliably inside the control it describes, so making the
// caller derive the click point is both a token cost and a source of misses.
//
// Geometry does the work that tree structure normally would. protocol.UITreeNode
// is a flat list with no parent link, but every node's bounds are in the same
// coordinate space the input verbs click in, so containment substitutes for
// ancestry and no client change is required to get this far.
package uitree

import (
	"sort"
	"strings"

	"abacad/internal/protocol"
)

// MaxTargets is the default cap on a returned target list.
//
// 255 is not an arbitrary round number: it is the per-choice cardinality ceiling
// of typed decision models, above which a caller has to fall back to a slower
// two-stage score-then-choose. Real screens are nowhere near it — the densest
// measured above yielded 24 — so the cap is a guard against a pathological tree,
// not a limit anything normal is expected to reach.
const MaxTargets = 255

// Target is one thing on screen an agent can act on.
type Target struct {
	// Index is the position in this snapshot, assigned in reading order. It is
	// meaningful only within the snapshot that produced it: the next capture
	// renumbers from scratch, so an index must never be held across frames.
	Index int    `json:"i"`
	Label string `json:"label,omitempty"`
	Role  string `json:"role,omitempty"`
	// X and Y are the center of the control — click or tap here directly.
	X int `json:"x"`
	Y int `json:"y"`
}

// Stats reports what each pass removed, so a caller can log the reduction
// instead of guessing at it.
type Stats struct {
	Nodes     int // nodes in the raw tree
	Visible   int // survived the visibility pass
	Clickable int // visible and flagged clickable
	Labelled  int // of those, the ones that ended up with a usable label
	Content   int // visible, not flagged clickable, promoted as addressable content
	Targets   int // returned, after dedup and cap
	Truncated int // dropped by the cap; non-zero means the caller is seeing a subset
}

// Extract reduces tree to the visible, actionable targets on a screenW x screenH
// screen, in reading order, capped at max (<= 0 means no cap).
//
// A zero or negative screen dimension disables the corresponding off-screen
// check rather than rejecting every node, so a device that does not report its
// screen size still gets the zero-area and labelling passes.
func Extract(tree *protocol.UITree, screenW, screenH, max int) ([]Target, Stats) {
	var st Stats
	if tree == nil {
		return nil, st
	}
	st.Nodes = len(tree.Nodes)

	// Pass 1. Everything downstream reads from the visible set, including the
	// text a label may be borrowed from: an invisible label is no more useful
	// than an invisible button.
	visible := make([]protocol.UITreeNode, 0, len(tree.Nodes))
	for _, n := range tree.Nodes {
		if onScreen(n.Bounds, screenW, screenH) {
			visible = append(visible, n)
		}
	}
	st.Visible = len(visible)

	texts := make([]protocol.UITreeNode, 0, len(visible))
	for _, n := range visible {
		if strings.TrimSpace(n.Text) != "" {
			texts = append(texts, n)
		}
	}

	// Pass 2.
	clickable := make([]protocol.UITreeNode, 0, 32)
	for _, n := range visible {
		if n.Clickable {
			clickable = append(clickable, n)
		}
	}
	st.Clickable = len(clickable)

	// Text sitting inside some control belongs to that control and is not
	// available to a neighbour, or a switch would steal the words off the button
	// next to it. Only unclaimed text can be borrowed sideways.
	unclaimed := make([]protocol.UITreeNode, 0, len(texts))
	for _, t := range texts {
		owned := false
		for _, c := range clickable {
			if contains(c.Bounds, t.Bounds) {
				owned = true
				break
			}
		}
		if !owned {
			unclaimed = append(unclaimed, t)
		}
	}

	type candidate struct {
		label, role string
		bounds      [4]int
	}
	candidates := make([]candidate, 0, len(clickable))
	borrowed := make([]bool, len(unclaimed))

	// Own words first, then words from inside. Whatever is still nameless after
	// those two is what the sideways borrow exists for.
	named := make([]string, len(clickable))
	for i, n := range clickable {
		named[i] = strings.TrimSpace(n.Text)
		if named[i] == "" {
			named[i] = innerLabel(n.Bounds, texts)
		}
	}
	for i, n := range clickable {
		if named[i] != "" || !onlyNamelessOnRow(clickable, named, i) {
			continue
		}
		if j := rowLabelIndex(n.Bounds, unclaimed, borrowed); j >= 0 {
			named[i], borrowed[j] = strings.TrimSpace(unclaimed[j].Text), true
		}
	}
	for i, n := range clickable {
		if named[i] != "" {
			st.Labelled++
		}
		candidates = append(candidates, candidate{label: named[i], role: n.Cls, bounds: n.Bounds})
	}

	// Pass 2b. Content that names itself but carries no clickable flag.
	//
	// The flag is not a reliable account of what can be acted on. macOS reports
	// a desktop icon as an AXImage supporting no AXPress, so the six icons on a
	// plain desktop are all dropped and the ten surviving targets are eight
	// menu-bar items, a stray radio button and Tags… — none of them the thing a
	// caller asked for. A click at the icon's center works regardless; the
	// accessibility flag is simply wrong about it.
	//
	// Three conditions together separate content from the captions and
	// containers that must stay out. It must not have been borrowed as some
	// control's label, or a settings row would yield both its words and its
	// switch. It must not be a caption role, or every heading on screen becomes
	// a target. And it must not enclose a smaller element, which is what rules
	// out the desktop's own AXScrollArea and AXGroup — both carry the text
	// "desktop" and both wrap everything else.
	for i, n := range unclaimed {
		if borrowed[i] || isCaption(n.Cls) || encloses(n.Bounds, visible) {
			continue
		}
		st.Content++
		candidates = append(candidates, candidate{label: strings.TrimSpace(n.Text), role: n.Cls, bounds: n.Bounds})
	}

	// Pass 3. Keying on the click point rather than the bounds also folds nested
	// wrappers together: a button and the container drawn tightly around it share
	// a center, and clicking either does the same thing.
	type key struct {
		label string
		x, y  int
	}
	seen := make(map[key]bool, len(candidates))
	targets := make([]Target, 0, len(candidates))
	for _, c := range candidates {
		x := (c.bounds[0] + c.bounds[2]) / 2
		y := (c.bounds[1] + c.bounds[3]) / 2
		k := key{label: c.label, x: x, y: y}
		if seen[k] {
			continue
		}
		seen[k] = true
		targets = append(targets, Target{Label: c.label, Role: c.role, X: x, Y: y})
	}

	// Reading order: top to bottom, then left to right. Indices are assigned
	// after sorting so they run the way a person scans the screen, which makes a
	// truncated list lose the bottom of the screen rather than an arbitrary slice.
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].Y != targets[j].Y {
			return targets[i].Y < targets[j].Y
		}
		return targets[i].X < targets[j].X
	})
	if max > 0 && len(targets) > max {
		st.Truncated = len(targets) - max
		targets = targets[:max]
	}
	for i := range targets {
		targets[i].Index = i
	}
	st.Targets = len(targets)
	return targets, st
}

// onScreen reports whether b describes a box a person could actually see and
// hit. Zero-area is the dominant case (closed menus, collapsed sections); wholly
// off-screen is the rest (a list's cached rows, a hidden drawer).
func onScreen(b [4]int, screenW, screenH int) bool {
	if b[2]-b[0] <= 0 || b[3]-b[1] <= 0 {
		return false
	}
	if b[2] <= 0 || b[3] <= 0 {
		return false
	}
	if screenW > 0 && b[0] >= screenW {
		return false
	}
	if screenH > 0 && b[1] >= screenH {
		return false
	}
	return true
}

// innerLabel returns the most specific visible text inside b. Most specific
// means smallest: a list row often contains both a title and a subtitle, and the
// tighter box is the one whose words name the control rather than describe it.
func innerLabel(b [4]int, texts []protocol.UITreeNode) string {
	best := ""
	bestArea := 0
	for _, t := range texts {
		if !contains(b, t.Bounds) {
			continue
		}
		a := area(t.Bounds)
		if best == "" || a < bestArea {
			best, bestArea = strings.TrimSpace(t.Text), a
		}
	}
	return best
}

// rowLabelIndex points at the text naming a control that holds none of its own:
// the nearest same-line text nobody has taken yet, or -1 when there is none.
// The index is returned rather than the string so the caller can mark that text
// spoken for — which keeps a second control from taking the same words, and
// keeps pass 2b from offering them again as a target of their own. This is the settings-row shape: the switch is
// the clickable thing and its words sit at the far end of the row, outside its
// bounds entirely. Nearest-on-the-line is what a person reads, and restricting
// the pool to unclaimed text keeps it from poaching another control's label.
func rowLabelIndex(b [4]int, unclaimed []protocol.UITreeNode, borrowed []bool) int {
	best, bestGap := -1, 0
	for i, t := range unclaimed {
		if borrowed[i] || !sameRow(b, t.Bounds) {
			continue
		}
		g := horizontalGap(b, t.Bounds)
		if best < 0 || g < bestGap {
			best, bestGap = i, g
		}
	}
	return best
}

// isCaption reports whether a role names a pure text element. A caption
// describes something else rather than being a thing in its own right, and it
// is the pool a neighbouring control borrows its words from — so promoting
// captions as well would put a label and the control it names on screen as two
// separate targets.
func isCaption(role string) bool {
	r := strings.ToLower(role)
	return r == "text" ||
		strings.HasSuffix(r, "statictext") ||
		strings.HasSuffix(r, "textview") ||
		strings.HasSuffix(r, "label")
}

// encloses reports whether b wraps a smaller visible element. Something that
// holds other elements is a region rather than a target: its words name the
// area, and a click at its center lands on whatever happens to sit there.
func encloses(b [4]int, visible []protocol.UITreeNode) bool {
	for _, n := range visible {
		if area(n.Bounds) < area(b) && contains(b, n.Bounds) {
			return true
		}
	}
	return false
}

// onlyNamelessOnRow reports whether clickable[i] is the one control on its line
// still waiting for a name.
//
// Words on a row name a single control, so when several nameless controls share
// that row nothing says which one they belong to. A Finder title bar is the
// case that matters: three anonymous traffic-light buttons sit on the same line
// as the window title, and letting each take the nearest words produced four
// targets called "archived" — one of which closes the window. Handing the text
// to the closest one is no better, because the losers then reach further out
// and come back with something worse. Leaving them all anonymous is the rule
// this package already states: two targets sharing a name is worse than one
// having none.
func onlyNamelessOnRow(clickable []protocol.UITreeNode, named []string, i int) bool {
	for k := range clickable {
		if k != i && named[k] == "" && sameRow(clickable[i].Bounds, clickable[k].Bounds) {
			return false
		}
	}
	return true
}

// sameRow reports whether two boxes sit on the same visual line, defined as
// their vertical spans overlapping by at least half of the shorter one. A plain
// center-point test misreads a tall row against a short label.
func sameRow(a, b [4]int) bool {
	overlap := min(a[3], b[3]) - max(a[1], b[1])
	if overlap <= 0 {
		return false
	}
	shorter := min(a[3]-a[1], b[3]-b[1])
	return overlap*2 >= shorter
}

// horizontalGap is the distance between two boxes along x, and 0 when they
// overlap.
func horizontalGap(a, b [4]int) int {
	if gap := b[0] - a[2]; gap > 0 {
		return gap
	}
	if gap := a[0] - b[2]; gap > 0 {
		return gap
	}
	return 0
}

func contains(outer, inner [4]int) bool {
	return inner[0] >= outer[0] && inner[1] >= outer[1] &&
		inner[2] <= outer[2] && inner[3] <= outer[3]
}

func area(b [4]int) int { return (b[2] - b[0]) * (b[3] - b[1]) }
