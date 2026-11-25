package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/danielpaulus/go-ios/ios"

	"iosvnc/internal/frames"
)

// check is one prerequisite, reported with the repair hint when it fails.
type check struct {
	name     string
	detail   string
	ok       bool
	required bool
	hint     string
}

// runDoctor walks every prerequisite in the order it can fail, so bringing up a
// device on a fresh Mac names the one thing that is missing instead of surfacing
// a CoreDevice error code.
func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	selector := fs.String("device", "", "device UDID or name fragment")
	tunnelInfo := fs.String("tunnel-info", "127.0.0.1:28100", "go-ios tunnel info address")
	_ = fs.Parse(args)

	checks := []check{
		checkMacOS(),
		checkXcode(),
		checkUsbmuxd(),
		checkTunnel(*tunnelInfo),
	}

	device, deviceErr := resolveDevice(*selector)
	if deviceErr != nil {
		checks = append(checks, check{
			name:     "device",
			detail:   shortError(deviceErr),
			ok:       false,
			required: true,
			hint:     "plug the iPhone in with a cable, unlock it, tap Trust, or enable Wi-Fi sync in Finder",
		})
	} else {
		name, product, version := deviceLabels(device)
		checks = append(checks, check{
			name:     "device",
			detail:   fmt.Sprintf("%s (%s, iOS %s)", name, product, version),
			ok:       true,
			required: true,
		})
		checks = append(checks, checkCapture(device))
	}

	printChecks(checks)

	failed := 0
	for _, c := range checks {
		if !c.ok && c.required {
			failed++
		}
	}
	if failed > 0 {
		fmt.Printf("\ndoctor: %d prerequisite(s) missing\n", failed)
		os.Exit(1)
	}
	fmt.Println("\ndoctor: every prerequisite is in place - now run 'iosvnc selftest'")
}

func printChecks(checks []check) {
	width := 0
	for _, c := range checks {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	fmt.Println("iosvnc doctor")
	fmt.Println()
	for _, c := range checks {
		status := "ok  "
		if !c.ok {
			if c.required {
				status = "FAIL"
			} else {
				status = "warn"
			}
		}
		fmt.Printf("  %s  %-*s  %s\n", status, width, c.name, c.detail)
		if !c.ok && c.hint != "" {
			fmt.Printf("        %-*s  -> %s\n", width, "", c.hint)
		}
	}
}

func checkMacOS() check {
	out, err := exec.Command("sw_vers", "-ProductVersion").Output()
	if err != nil {
		return check{name: "macOS", detail: "cannot read the version", ok: false, required: true}
	}
	return check{name: "macOS", detail: strings.TrimSpace(string(out)), ok: true, required: true}
}

// checkXcode looks for the version that ships Device Hub and the CoreDevice
// developer services this bridge drives.
func checkXcode() check {
	out, err := exec.Command("xcodebuild", "-version").Output()
	if err != nil {
		return check{
			name:     "Xcode",
			detail:   "not installed or not selected",
			ok:       false,
			required: true,
			hint:     "install Xcode 27 and run: sudo xcode-select -s /Applications/Xcode.app",
		}
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	version := strings.TrimPrefix(line, "Xcode ")
	major := version
	if i := strings.IndexByte(major, '.'); i > 0 {
		major = major[:i]
	}
	if major < "27" {
		return check{
			name:     "Xcode",
			detail:   fmt.Sprintf("%s (27 is needed for the CoreDevice developer services)", version),
			ok:       false,
			required: true,
			hint:     "upgrade Xcode to 27 or later",
		}
	}
	return check{name: "Xcode", detail: version, ok: true, required: true}
}

func checkUsbmuxd() check {
	const socket = "/var/run/usbmuxd"
	if _, err := os.Stat(socket); err != nil {
		return check{
			name:     "usbmuxd",
			detail:   "not running",
			ok:       false,
			required: true,
			hint:     "unplug and replug the device; the daemon starts with it",
		}
	}
	return check{name: "usbmuxd", detail: socket, ok: true, required: true}
}

// checkTunnel asks the go-ios tunnel daemon whether it is up. Everything behind
// the developer disk image is unreachable without it.
func checkTunnel(addr string) check {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/ready")
	if err != nil {
		return check{
			name:     "tunnel",
			detail:   "no daemon at " + addr,
			ok:       false,
			required: true,
			hint:     `run in another terminal: sudo "$(go env GOPATH)/bin/ios" tunnel start`,
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return check{
			name:     "tunnel",
			detail:   fmt.Sprintf("daemon at %s is not ready (%s)", addr, resp.Status),
			ok:       false,
			required: true,
			hint:     "wait for it to finish pairing, or restart it",
		}
	}
	tunnels := parseTunnels(body)
	return check{name: "tunnel", detail: tunnels, ok: true, required: true}
}

// parseTunnels summarises the daemon's tunnel list without failing on a shape we
// do not expect: the count is cosmetic.
func parseTunnels(body []byte) string {
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		return "ready"
	}
	if len(list) == 0 {
		return "ready, no tunnels yet"
	}
	return fmt.Sprintf("ready, %d tunnel(s)", len(list))
}

// checkCapture is the definitive test of the developer disk image: it either
// answers with a frame or it does not.
func checkCapture(device ios.DeviceEntry) check {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	type result struct {
		src *frames.Screenshot
		err error
	}
	ch := make(chan result, 1)
	go func() {
		src, err := frames.OpenScreenshot(device, 0)
		ch <- result{src, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return check{
				name:     "capture",
				detail:   shortError(r.err),
				ok:       false,
				required: true,
				hint:     "unlock the device and keep it unlocked; the developer disk image will not mount on a locked phone",
			}
		}
		w, h := r.src.Size()
		r.src.Close()
		return check{name: "capture", detail: fmt.Sprintf("%dx%d frame", w, h), ok: true, required: true}
	case <-ctx.Done():
		return check{
			name:     "capture",
			detail:   "timed out after 20s",
			ok:       false,
			required: true,
			hint:     "the device is busy or the tunnel is unhealthy; try again after unlocking it",
		}
	}
}

// shortError keeps a CoreDevice error readable on one line.
func shortError(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > 160 {
		msg = msg[:157] + "..."
	}
	return msg
}
