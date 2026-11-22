package inventory

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"abacad/internal/config"
	"abacad/internal/rfb"
)

const (
	// vncProbeMinInterval is the floor between authenticated RFB probes. A
	// successful authentication does not count as a security failure, but it is
	// still a real client connection, so keep it rare.
	vncProbeMinInterval = 5 * time.Minute
	// vncMaxAuthFailures is deliberately below TigerVNC's default
	// BlacklistThreshold of 5. After two authentication failures the hub stops
	// automatic probes for vncFailureCooldown, so a stale password cannot
	// blacklist the hub's IP.
	vncMaxAuthFailures = 2
	vncFailureCooldown = time.Hour
	vncMaxBackoff      = time.Hour

	commandInterval   = 60 * time.Second
	commandMaxBackoff = 15 * time.Minute
	commandTimeout    = 10 * time.Second

	// staleAfter is how long a session-derived "online" state stays credible
	// without a new authenticated session. After that the UI says "not checked"
	// rather than pretending the device is still online.
	staleAfter    = 10 * time.Minute
	schedulerTick = 5 * time.Second
)

// Run schedules only the health checks a device explicitly opted into. The
// default ("session") opens no connections at all; the old 30-second bare TCP
// probe is gone because TigerVNC counts every unauthenticated close as a
// failure and never decays the counter, which permanently blacklists the hub.
func (in *Inventory) Run(ctx context.Context) {
	t := time.NewTicker(schedulerTick)
	defer t.Stop()
	in.runDue(ctx)
	in.expireStale()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			in.runDue(ctx)
			in.expireStale()
		}
	}
}

func (in *Inventory) runDue(ctx context.Context) {
	devices := in.Devices()
	sem := make(chan struct{}, 4)
	for _, v := range devices {
		d := v.Device
		if healthMode(d) == config.HealthSession {
			continue
		}
		in.mu.Lock()
		st := in.status[d.ID]
		now := time.Now().UTC()
		if st.Checking || (!st.NextCheck.IsZero() && now.Before(st.NextCheck)) {
			in.mu.Unlock()
			continue
		}
		st.Checking = true
		in.status[d.ID] = st
		in.mu.Unlock()

		sem <- struct{}{}
		go func(d config.Device) {
			defer func() { <-sem }()
			in.runCheck(ctx, d)
		}(d)
	}
}

func (in *Inventory) expireStale() {
	in.mu.Lock()
	defer in.mu.Unlock()
	now := time.Now().UTC()
	for _, d := range in.cfg.Devices {
		// Command and VNC health checks have their own schedule; only passive
		// session-derived state needs expiring.
		if healthMode(d) != config.HealthSession {
			continue
		}
		st := in.status[d.ID]
		if st.Known && !st.Checking && !st.LastSeen.IsZero() && now.Sub(st.LastSeen) > staleAfter {
			st.Known = false
			st.Online = false
			in.status[d.ID] = st
		}
	}
}

// checkDevice is the manual path (the card's Check button, and Add/Update).
// It never opens a bare TCP connection: either it runs the configured health
// command, or it performs a full authenticated RFB handshake.
func (in *Inventory) checkDevice(ctx context.Context, id string, manual bool) Status {
	d, ok := in.configDevice(id)
	if !ok {
		return Status{LastError: "device not found"}
	}

	in.mu.Lock()
	st := in.status[id]
	now := time.Now().UTC()
	if st.Checking {
		in.mu.Unlock()
		return st
	}

	// A command check has no effect on the device's blacklist, so it can run on
	// demand. A VNC check is a real connection and is rate-limited.
	if healthMode(d) != config.HealthCommand {
		if !st.LastCheck.IsZero() && now.Sub(st.LastCheck) < vncProbeMinInterval {
			next := st.LastCheck.Add(vncProbeMinInterval)
			st.Throttled = true
			st.ThrottleMessage = fmt.Sprintf(
				"VNC health checks are limited to one every %s to protect the device; next check at %s",
				vncProbeMinInterval, next.Local().Format("15:04:05"),
			)
			in.mu.Unlock()
			return st
		}
		if st.AuthFailures >= vncMaxAuthFailures && now.Before(st.NextCheck) {
			st.Throttled = true
			st.ThrottleMessage = st.LastError
			if st.ThrottleMessage == "" {
				st.ThrottleMessage = "VNC health checks are paused after repeated authentication failures; fix the credential or wait"
			}
			in.mu.Unlock()
			return st
		}
	}
	st.Checking = true
	in.status[id] = st
	in.mu.Unlock()

	return in.runCheck(ctx, d)
}

func (in *Inventory) runCheck(ctx context.Context, d config.Device) Status {
	in.mu.RLock()
	st := in.status[d.ID]
	in.mu.RUnlock()

	if healthMode(d) == config.HealthCommand {
		return in.checkCommand(ctx, d, st)
	}
	return in.checkVNC(ctx, d, st)
}

// healthMode resolves legacy configs where health_command was set without an
// explicit health mode. An explicit mode always wins, so switching to
// "session" cannot accidentally keep running an old command.
func healthMode(d config.Device) string {
	if d.Health != "" {
		return d.Health
	}
	if strings.TrimSpace(d.HealthCommand) != "" {
		return config.HealthCommand
	}
	return config.HealthSession
}

func (in *Inventory) checkCommand(ctx context.Context, d config.Device, st Status) Status {
	output, err := runHealthCommand(ctx, d.HealthCommand)
	now := time.Now().UTC()
	st.Checking = false
	st.LastCheck = now
	st.Known = true
	st.Detail = output

	if err == nil {
		st.Online = true
		st.LastSeen = now
		st.LastError = ""
		st.Warning = ""
		st.Failures = 0
		st.AuthFailures = 0
		st.NextCheck = now.Add(commandInterval)
	} else {
		st.Online = false
		st.LastError = err.Error()
		st.Failures++
		st.NextCheck = now.Add(exponentialBackoff(commandInterval, st.Failures, commandMaxBackoff))
	}

	in.store(d.ID, st)
	return st
}

func (in *Inventory) checkVNC(ctx context.Context, d config.Device, st Status) Status {
	result, err := rfb.Probe(ctx, rfb.ProbeOptions{
		Addr:     net.JoinHostPort(d.Host, strconv.Itoa(d.Port)),
		Password: d.Password,
		Username: d.Username,
		ARD:      d.Auth == config.AuthARD,
		Timeout:  5 * time.Second,
	})
	now := time.Now().UTC()
	st.Checking = false
	st.LastCheck = now
	st.Known = true
	if result.Banner != "" {
		st.Banner = strings.TrimSpace(result.Banner)
	}

	if err == nil {
		st.Online = true
		st.LastSeen = now
		st.LastError = ""
		st.Failures = 0
		st.AuthFailures = 0
		st.Warning = warningFor(d, result.Types, result.Banner)
		st.Detail = describeProbeResult(result)
		st.NextCheck = now.Add(vncProbeMinInterval)
	} else {
		st.Online = false
		st.Detail = ""
		st.LastError = describeError(err)
		st.Failures++
		if rfb.IsAuthFailure(err) {
			st.AuthFailures++
		}
		if len(result.Types) > 0 {
			st.Warning = warningFor(d, result.Types, result.Banner)
		}

		switch {
		case rfb.IsTooManySecurityFailures(err):
			// The server has already blacklisted this IP. Retrying now only
			// extends the lockout; wait out the cooldown.
			st.AuthFailures = vncMaxAuthFailures
			st.NextCheck = now.Add(vncFailureCooldown)
		case st.AuthFailures >= vncMaxAuthFailures:
			st.LastError += fmt.Sprintf(" (automatic checks paused for %s)", vncFailureCooldown)
			st.NextCheck = now.Add(vncFailureCooldown)
		default:
			st.NextCheck = now.Add(exponentialBackoff(vncProbeMinInterval, st.Failures, vncMaxBackoff))
		}
	}

	in.store(d.ID, st)
	return st
}

func (in *Inventory) store(id string, st Status) {
	in.mu.Lock()
	in.status[id] = st
	in.mu.Unlock()
}

func (in *Inventory) configDevice(id string) (config.Device, bool) {
	in.mu.RLock()
	defer in.mu.RUnlock()
	for _, d := range in.cfg.Devices {
		if d.ID == id {
			return d, true
		}
	}
	return config.Device{}, false
}

func exponentialBackoff(base time.Duration, failures int, max time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	delay := base
	for i := 1; i < failures; i++ {
		delay *= 2
		if delay >= max {
			return max
		}
	}
	if delay > max {
		delay = max
	}
	return delay
}

func runHealthCommand(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("health command is empty")
	}
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > 2000 {
		text = text[:2000]
	}
	text = firstLine(text)
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			msg := fmt.Sprintf("timed out after %s", commandTimeout)
			if text != "" {
				msg += ": " + text
			}
			return "", errors.New(msg)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := fmt.Sprintf("exit status %d", exitErr.ExitCode())
			if text != "" {
				msg += ": " + text
			}
			return "", errors.New(msg)
		}
		return "", err
	}
	if text == "" {
		text = "ok"
	}
	return text, nil
}

func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func describeProbeResult(r rfb.ProbeResult) string {
	if r.DesktopName != "" {
		return fmt.Sprintf("%s (%dx%d)", r.DesktopName, r.Width, r.Height)
	}
	return fmt.Sprintf("%dx%d", r.Width, r.Height)
}

func warningFor(d config.Device, types []byte, banner string) string {
	if len(types) == 0 {
		return ""
	}
	switch d.Auth {
	case config.AuthARD:
		if !rfb.SupportsARD(types) {
			return fmt.Sprintf("device offers no macOS account sign-in (security types %v)", types)
		}
	default:
		if !rfb.SupportsStandardAuth(types) {
			return fmt.Sprintf("device only offers security types %v; enable \"VNC viewers may control screen with password\" or use TigerVNC", types)
		}
		if rfb.SupportsARD(types) && (d.Platform == "macos" || strings.Contains(banner, "003.889")) {
			return "macOS: switch Sign-in to \"macOS account\". The legacy VNC password has no user identity and can land on the login window."
		}
	}
	return ""
}

// HealthInterval is exposed for tests and documentation. It is the minimum
// spacing between automatic authenticated RFB probes.
func HealthInterval() time.Duration { return vncProbeMinInterval }
