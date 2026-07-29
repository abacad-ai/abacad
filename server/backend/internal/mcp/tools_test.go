package mcp

import (
	"testing"

	"abacad/internal/protocol"
)

// allowScope permits exactly the protocol methods it lists.
type allowScope map[string]bool

func (s allowScope) AllowsMethod(name string) bool { return s[name] }

// TestToolMethodsMatchProtocol pins the mapping every scope check depends on.
// An API key's allowlist is validated against protocol.Methods (api.knownMethods)
// but tools are addressed by tool name, and the two names differ for file
// transfer. If the sets ever drift apart again, a method becomes either
// ungrantable (stored name never matches a tool) or unguardable (tool has no
// method to authorize against), so assert a strict bijection in both directions.
func TestToolMethodsMatchProtocol(t *testing.T) {
	byMethod := make(map[protocol.Method]string, len(actionTools))
	for _, tool := range actionTools {
		if tool.method == "" {
			t.Errorf("tool %q has no method — it cannot be scoped", tool.name)
			continue
		}
		if prev, dup := byMethod[tool.method]; dup {
			t.Errorf("tools %q and %q both claim method %q", prev, tool.name, tool.method)
		}
		byMethod[tool.method] = tool.name
	}

	known := make(map[protocol.Method]bool, len(protocol.Methods))
	for _, m := range protocol.Methods {
		known[m] = true
		if _, ok := byMethod[m]; !ok {
			t.Errorf("protocol method %q has no tool — it can be granted but never called", m)
		}
	}
	for m, name := range byMethod {
		if !known[m] {
			t.Errorf("tool %q maps to %q, which is not in protocol.Methods — it can never be granted", name, m)
		}
	}
}

// TestFileTransferIsGrantable is the regression for the bug this mapping fixes.
// The scope check used the tool name ("send_file") while the allowlist validator
// only ever accepted protocol names ("push_file"), so the two could never agree
// and file transfer was reachable only with all_methods. A key scoped to exactly
// push_file/pull_file must be able to call send_file/get_file — and nothing else.
func TestFileTransferIsGrantable(t *testing.T) {
	scope := allowScope{
		string(protocol.MethodPushFile): true,
		string(protocol.MethodPullFile): true,
	}

	for _, name := range []string{"send_file", "get_file"} {
		tool, ok := actionByName[name]
		if !ok {
			t.Fatalf("no such tool: %s", name)
		}
		if !scope.AllowsMethod(string(tool.method)) {
			t.Errorf("%s not permitted by a scope granting push_file/pull_file", name)
		}
	}

	if tool := actionByName["screenshot"]; scope.AllowsMethod(string(tool.method)) {
		t.Error("screenshot permitted by a file-transfer-only scope")
	}

	// tools/list must agree with tools/call, or the agent sees tools it cannot
	// invoke (or, worse, is hidden ones it can).
	listed := make(map[string]bool)
	for _, info := range toolInfos(scope) {
		listed[info.Name] = true
	}
	for _, name := range []string{"send_file", "get_file"} {
		if !listed[name] {
			t.Errorf("%s missing from tools/list under a scope that permits it", name)
		}
	}
	if listed["screenshot"] {
		t.Error("screenshot listed under a file-transfer-only scope")
	}
}
