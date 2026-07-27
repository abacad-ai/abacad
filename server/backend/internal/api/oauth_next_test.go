package api

import "testing"

// TestSafeNextPath pins the open-redirect guard on ?next=. The sign-in flow
// accepts a post-auth destination so a claim started before signing in resumes
// afterwards; without this filter that parameter would let any link turn our
// own sign-in page into a redirector to an attacker's site.
func TestSafeNextPath(t *testing.T) {
	ok := []string{
		"/",
		"/claim",
		"/claim?d=abcdefghijklmnop&c=WXYZ-2K7M",
		"/devices/abcdefghijklmnop",
	}
	for _, in := range ok {
		if got := safeNextPath(in); got != in {
			t.Errorf("safeNextPath(%q) = %q, want it preserved", in, got)
		}
	}

	bad := []string{
		"",                             // absent
		"//evil.example.com",           // protocol-relative — browsers follow this off-site
		"///evil.example.com",          //
		"https://evil.example.com",     // absolute
		"http://evil.example.com/x",    //
		"javascript:alert(1)",          // scheme, not a path
		"claim",                        // relative, not rooted
		"/claim\r\nSet-Cookie: a=b",    // response splitting
		"/claim\nLocation: /elsewhere", //
	}
	for _, in := range bad {
		if got := safeNextPath(in); got != "" {
			t.Errorf("safeNextPath(%q) = %q, want \"\" (rejected)", in, got)
		}
	}

	// Absurdly long values are rejected rather than reflected into a header.
	long := "/" + string(make([]byte, 600))
	if got := safeNextPath(long); got != "" {
		t.Errorf("overlong next should be rejected, got %q", got)
	}
}
