package agent

import (
	"encoding/json"
	"testing"
)

func TestParamAccessorsFromJSON(t *testing.T) {
	// JSON numbers decode as float64 — the case paramInt must survive.
	var m map[string]any
	if err := json.Unmarshal([]byte(`{
		"x": 42, "flag": true, "text": "hi",
		"keys": ["ctrl", "c"],
		"steps": [{"op": "wait", "ms": 5}]
	}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := paramInt(m, "x", -1); got != 42 {
		t.Errorf("paramInt x = %d, want 42", got)
	}
	if got := paramInt(m, "missing", 7); got != 7 {
		t.Errorf("paramInt default = %d, want 7", got)
	}
	if !paramBool(m, "flag", false) {
		t.Errorf("paramBool flag = false, want true")
	}
	if got := paramStr(m, "text", ""); got != "hi" {
		t.Errorf("paramStr text = %q, want hi", got)
	}
	if got := paramStrs(m, "keys"); len(got) != 2 || got[0] != "ctrl" || got[1] != "c" {
		t.Errorf("paramStrs keys = %v, want [ctrl c]", got)
	}
	steps := paramObjs(m, "steps")
	if len(steps) != 1 || paramStr(steps[0], "op", "") != "wait" || paramInt(steps[0], "ms", 0) != 5 {
		t.Errorf("paramObjs steps = %v", steps)
	}
}
