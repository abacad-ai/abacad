// Package discovery finds VNC servers on the local network via DNS-SD/mDNS.
//
// Discovery is a convenience, never an authorization: an entry is just a hint
// until a human adds it and the hub successfully authenticates. Not every VNC
// server advertises _rfb._tcp, so adding by host:port always works.
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/grandcat/zeroconf"
)

type Entry struct {
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Addresses []string `json:"addresses,omitempty"`
	Source    string   `json:"source"`
}

type Discovery struct {
	log *slog.Logger

	mu      sync.RWMutex
	entries map[string]Entry
	server  *zeroconf.Server
	cancel  context.CancelFunc
}

func New(log *slog.Logger) *Discovery {
	return &Discovery{log: log, entries: map[string]Entry{}}
}

// Start advertises the hub and browses for _rfb._tcp services. It returns
// immediately; discovery failures are logged, not fatal.
func (d *Discovery) Start(ctx context.Context, servicePort int, instance string) {
	ctx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	if servicePort > 0 {
		txt := []string{"path=/", "version=0.1.0"}
		srv, err := zeroconf.Register(instance, "_abacad._tcp", "local.", servicePort, txt, nil)
		if err != nil {
			d.log.Warn("mdns advertise failed", "err", err)
		} else {
			d.server = srv
			d.log.Info("mdns advertised", "service", "_abacad._tcp", "port", servicePort)
		}
	}

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		d.log.Warn("mdns resolver failed", "err", err)
		return
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for e := range entries {
			d.observe(e)
		}
	}()
	go func() {
		if err := resolver.Browse(ctx, "_rfb._tcp", "local.", entries); err != nil {
			d.log.Warn("mdns browse failed", "err", err)
		}
	}()
}

func (d *Discovery) observe(e *zeroconf.ServiceEntry) {
	if e.Port <= 0 {
		return
	}
	var addrs []string
	for _, ip := range e.AddrIPv4 {
		addrs = append(addrs, ip.String())
	}
	host := ""
	if len(addrs) > 0 {
		host = addrs[0]
	} else if len(e.AddrIPv6) > 0 {
		host = e.AddrIPv6[0].String()
	} else {
		host = strings.TrimSuffix(e.HostName, ".")
	}
	key := fmt.Sprintf("%s|%s|%d", e.Instance, host, e.Port)
	entry := Entry{Name: strings.TrimSuffix(unescapeDNS(e.Instance), "."), Host: host, Port: e.Port, Addresses: addrs, Source: "mdns"}
	d.mu.Lock()
	d.entries[key] = entry
	d.mu.Unlock()
	d.log.Debug("mdns discovered", "name", entry.Name, "host", host, "port", e.Port)
}

// unescapeDNS turns zeroconf's \DDD octal escapes back into UTF-8. macOS
// advertises names containing an apostrophe as escapes, and a UI that shows
// "Li\226\128\153s Mac mini" is not a nice first impression.
func unescapeDNS(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b []byte
	for i := 0; i < len(s); {
		if s[i] == '\\' {
			if i+3 < len(s) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
				if v, err := strconv.ParseUint(s[i+1:i+4], 10, 8); err == nil {
					b = append(b, byte(v))
					i += 4
					continue
				}
				b = append(b, '\\')
				i++
				continue
			}
			if i+1 < len(s) {
				b = append(b, s[i+1])
				i += 2
				continue
			}
		}
		b = append(b, s[i])
		i++
	}
	if utf8.Valid(b) {
		return string(b)
	}
	return s
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func (d *Discovery) Entries() []Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Entry, 0, len(d.entries))
	for _, e := range d.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Host < out[j].Host
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (d *Discovery) Close() {
	if d.cancel != nil {
		d.cancel()
	}
	if d.server != nil {
		d.server.Shutdown()
	}
}

// ListenPort extracts the port from a listen address such as 0.0.0.0:8080.
func ListenPort(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		if i, err2 := strconv.Atoi(strings.TrimPrefix(addr, ":")); err2 == nil {
			return i
		}
		return 0
	}
	i, _ := strconv.Atoi(p)
	return i
}

// LocalHostname is a stable-ish mDNS instance name.
func LocalHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "abacad"
	}
	return "abacad@" + h
}
