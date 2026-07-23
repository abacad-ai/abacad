package relay

import (
	"context"
	"errors"
	"testing"
)

// TestClassifyNeverLeaksParams pins a data-minimization invariant: the audit
// trail records a command's outcome, never its parameters (typed text, tap
// coordinates, file bytes, JS). classify is the only source of an activity row's
// free-form `detail`, so it must emit nothing content-bearing.
//
//   - success and the clean sentinels carry an EMPTY detail;
//   - the error paths carry only the sentinel/device error string — which is a
//     status message, never the command's params (classify is handed the error,
//     not the request).
func TestClassifyNeverLeaksParams(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		outcome    string
		wantDetail string
	}{
		{"success", nil, "ok", ""},
		{"timeout", ErrTimeout, "timeout", ""},
		{"device_gone", ErrDeviceGone, "device_gone", ""},
		{"canceled", context.Canceled, "canceled", "context canceled"},
		{"device_error", errors.New("element not found"), "error", "element not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outcome, detail := classify(tc.err)
			if outcome != tc.outcome {
				t.Errorf("outcome = %q, want %q", outcome, tc.outcome)
			}
			if detail != tc.wantDetail {
				t.Errorf("detail = %q, want %q", detail, tc.wantDetail)
			}
		})
	}
}
