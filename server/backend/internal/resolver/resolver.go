// Package resolver bridges an authenticated MCP account to the live device
// connections it may drive. Ownership is checked against the store; liveness
// against the relay hub. The two are separate so offline-but-owned yields a
// precise message instead of a blanket "no device".
package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"abacad/internal/mcp"
	"abacad/internal/relay"
	"abacad/internal/store"
)

// Factory builds a per-request resolver for one account.
type Factory struct {
	Store *store.Store
	Hub   *relay.Hub
}

// For returns the DeviceResolver scoped to accountID.
func (f *Factory) For(accountID string) mcp.DeviceResolver {
	return &accountResolver{store: f.Store, hub: f.Hub, accountID: accountID}
}

type accountResolver struct {
	store     *store.Store
	hub       *relay.Hub
	accountID string
}

// Resolve maps an optional device_id to a live connection the account owns.
func (a *accountResolver) Resolve(_ context.Context, deviceID string) (*relay.DeviceConn, error) {
	if deviceID != "" {
		d, err := a.store.DeviceOwnedBy(deviceID, a.accountID)
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("device %q is not in your account — call list_devices to see your devices", deviceID)
		}
		if err != nil {
			return nil, err
		}
		dc, ok := a.hub.Get(d.ID)
		if !ok {
			return nil, fmt.Errorf("device %q (%s) is not connected — open the Abacad app on it", d.Name, d.ID)
		}
		return dc, nil
	}

	// No device_id: pick the most-recently-active device that is online.
	devices, err := a.store.DevicesByAccount(a.accountID)
	if err != nil {
		return nil, err
	}
	for _, d := range devices { // already ordered last_seen desc
		if dc, ok := a.hub.Get(d.ID); ok {
			return dc, nil
		}
	}
	// Phrasing kept compatible with the smoke retry on /no device connected/.
	return nil, errors.New("no device connected — open the Abacad app on one of your devices and connect it, then try again (see list_devices)")
}

// List returns the account's devices with live status for the list_devices tool.
func (a *accountResolver) List(_ context.Context) ([]mcp.DeviceSummary, error) {
	devices, err := a.store.DevicesByAccount(a.accountID)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.DeviceSummary, 0, len(devices))
	for _, d := range devices {
		s := mcp.DeviceSummary{DeviceID: d.ID, Name: d.Name, Online: a.hub.Online(d.ID)}
		if d.LastSeen > 0 {
			s.LastSeen = time.Unix(d.LastSeen, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, s)
	}
	return out, nil
}
