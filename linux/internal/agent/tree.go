package agent

// emptyTree is the v1 placeholder for the accessibility tree. It returns the
// same flat shape the other clients produce ({pkg, nodes:[{cls,text,id,
// clickable,bounds}]}) so protocol.UITree decoding and agent reasoning stay
// uniform — just with no nodes yet.
//
// TODO(atspi): walk the focused application's AT-SPI tree over D-Bus (godbus)
// and emit real nodes. AT-SPI bounds are in the same root-window pixel space as
// capture/click, so populating this is additive — it changes no wire shape and
// no coordinate math.
func emptyTree() map[string]any {
	return map[string]any{"pkg": "", "nodes": []any{}}
}
