// Package events is an in-memory, per-device ring buffer of recent activity —
// connects, disconnects (with reason), and every command (tool, source,
// duration, outcome). It exists to answer the two questions the system couldn't
// before: "why did that MCP call time out?" and "why isn't the device
// connected?". Both are now a log line AND a queryable event.
//
// This is live-debugging state, not history: it lives only in memory, is capped
// per device, and resets on restart. Nothing here is on the hot path of a
// command (append is a mutex + slice op), so it is cheap enough to record every
// command unconditionally.
package events

import (
	"sync"
	"time"
)

// Kind categorizes an event.
type Kind string

const (
	KindConnected    Kind = "connected"
	KindDisconnected Kind = "disconnected"
	KindCommand      Kind = "command"
)

// Event is one entry in a device's activity log. Command fields are empty for
// connect/disconnect events; Detail carries the disconnect reason or a command's
// error message.
type Event struct {
	Ts         int64  `json:"ts"` // unix millis
	Kind       Kind   `json:"kind"`
	Method     string `json:"method,omitempty"`
	Source     string `json:"source,omitempty"` // agent | dashboard
	DurationMs int64  `json:"duration_ms,omitempty"`
	Outcome    string `json:"outcome,omitempty"` // ok | timeout | device_gone | canceled | error
	Detail     string `json:"detail,omitempty"`
}

// perDeviceCap bounds each device's ring buffer. ~200 keeps a useful window even
// while the dashboard polls a screenshot every ~10s, without unbounded growth.
const perDeviceCap = 200

// Log is the per-device ring buffer. Safe for concurrent use.
type Log struct {
	mu   sync.Mutex
	byID map[string][]Event
}

// NewLog returns an empty event log.
func NewLog() *Log { return &Log{byID: make(map[string][]Event)} }

// Append records one event for a device, stamping the time if unset and evicting
// the oldest event once the buffer is full.
func (l *Log) Append(deviceID string, e Event) {
	if e.Ts == 0 {
		e.Ts = time.Now().UnixMilli()
	}
	l.mu.Lock()
	buf := l.byID[deviceID]
	buf = append(buf, e)
	if len(buf) > perDeviceCap {
		// Drop the oldest, keeping the newest perDeviceCap.
		buf = append(buf[:0:0], buf[len(buf)-perDeviceCap:]...)
	}
	l.byID[deviceID] = buf
	l.mu.Unlock()
}

// Recent returns up to n most-recent events for a device, newest first. n <= 0
// returns them all.
func (l *Log) Recent(deviceID string, n int) []Event {
	l.mu.Lock()
	buf := l.byID[deviceID]
	if n > 0 && len(buf) > n {
		buf = buf[len(buf)-n:]
	}
	out := make([]Event, len(buf))
	// Reverse into out so the caller (and the dashboard) sees newest first.
	for i, e := range buf {
		out[len(buf)-1-i] = e
	}
	l.mu.Unlock()
	return out
}
