package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"abacad-linux/internal/capability"
)

// runCapabilities implements `abacad capabilities` — the device-side control
// over what this machine exposes.
//
//	abacad capabilities                       # show what is on and off
//	abacad capabilities --disable files,ssh   # turn groups or names off
//	abacad capabilities --enable screenshot   # turn them back on
//	abacad capabilities --only screenshot     # exactly this, nothing else
//	abacad capabilities --none                # expose nothing
//	abacad capabilities --all                 # expose everything (the default)
//
// Changes are written to ~/.config/abacad/capabilities and take effect on the
// running daemon at its next start. They are also reported to the relay, which
// mirrors them in the dashboard and stops sending what this device refuses —
// but the refusal does not depend on the relay agreeing.
func runCapabilities(args []string) {
	fs := flag.NewFlagSet("capabilities", flag.ExitOnError)
	var enable, disable, only string
	var all, none bool
	fs.StringVar(&enable, "enable", "", "comma-separated capabilities or groups to expose")
	fs.StringVar(&disable, "disable", "", "comma-separated capabilities or groups to stop exposing")
	fs.StringVar(&only, "only", "", "expose exactly these, nothing else")
	fs.BoolVar(&all, "all", false, "expose everything, including capabilities added in future versions")
	fs.BoolVar(&none, "none", false, "expose nothing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: abacad capabilities [--enable X] [--disable X] [--only X] [--all] [--none]\n\n")
		fmt.Fprintf(os.Stderr, "Groups: %s\n", strings.Join(groupNames(), ", "))
		fmt.Fprintf(os.Stderr, "Names:  %s\n\n", strings.Join(capability.All, ", "))
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	if err := capability.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "abacad: could not read capability config: %v\n", err)
		os.Exit(1)
	}

	changed := false
	switch {
	case all:
		capability.Set([]string{capability.Wildcard})
		changed = true
	case none:
		capability.Set(nil)
		changed = true
	case only != "":
		names, err := expand(only)
		if err != nil {
			fmt.Fprintf(os.Stderr, "abacad: %v\n", err)
			os.Exit(1)
		}
		capability.Set(names)
		changed = true
	}
	for _, spec := range []struct {
		list string
		on   bool
	}{{enable, true}, {disable, false}} {
		if spec.list == "" {
			continue
		}
		names, err := expand(spec.list)
		if err != nil {
			fmt.Fprintf(os.Stderr, "abacad: %v\n", err)
			os.Exit(1)
		}
		for _, n := range names {
			capability.Toggle(n, spec.on)
		}
		changed = true
	}

	if changed {
		if err := capability.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "abacad: could not save: %v\n", err)
			os.Exit(1)
		}
	}
	printStatus(changed)
}

// groups are the same groupings the dashboard shows, so a person moving between
// the two is choosing the same things by the same names.
var groups = map[string][]string{
	"observe": {capability.Screenshot, capability.ScreenRecording},
	"control": {
		capability.Tap, capability.LongPress, capability.Swipe, capability.InputText,
		capability.Back, capability.Home, capability.Recents,
		capability.Click, capability.RightClick, capability.Drag, capability.Scroll,
		capability.PressKeys, capability.Composite,
	},
	"files":   {capability.PushFile, capability.PullFile},
	"execute": {capability.Execute},
	"live":    {capability.VNC},
	"tunnel":  {capability.Tunnel},
	"ssh":     {capability.SSH},
}

func groupNames() []string {
	out := make([]string, 0, len(groups))
	for g := range groups {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// expand resolves a comma-separated list of group and capability names, failing
// on anything unrecognized rather than silently ignoring it — a typo that
// quietly did nothing would leave someone believing they had turned something
// off when they had not.
func expand(list string) ([]string, error) {
	var out []string
	for _, raw := range strings.Split(list, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if members, ok := groups[name]; ok {
			out = append(out, members...)
			continue
		}
		known := false
		for _, c := range capability.All {
			if c == name {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("unknown capability or group %q (groups: %s)", name, strings.Join(groupNames(), ", "))
		}
		out = append(out, name)
	}
	return out, nil
}

func printStatus(changed bool) {
	on := map[string]bool{}
	for _, c := range capability.Enabled() {
		on[c] = true
	}
	fmt.Println("This device exposes:")
	for _, c := range capability.All {
		mark := "off"
		if on[c] {
			mark = " on"
		}
		fmt.Printf("  %s  %s\n", mark, c)
	}
	if changed {
		fmt.Println("\nSaved. Restart the abacad daemon for this to take effect,")
		fmt.Println("and it will be reported to the relay on reconnect.")
	}
	// Say the sharp part out loud, in the place someone is actually making the
	// decision. A tunnel can dial this machine's own sshd, so "ssh off, tunnel
	// on" is not the closed door it looks like.
	if on[capability.Tunnel] && !on[capability.SSH] {
		fmt.Println("\nNote: ssh is off but tunnel is on — a tunnel can dial this machine's")
		fmt.Println("own port 22 directly, so ssh is still reachable. Disable tunnel too.")
	}
}
