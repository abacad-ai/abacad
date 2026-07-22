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

// For returns a full-access DeviceResolver for accountID (every device). Used by
// paths whose credential is not a scoped API key — today the SSH jump, which
// authenticates by public key and reaches all of the account's devices.
func (f *Factory) For(accountID string) mcp.DeviceResolver {
	return f.ForScope(accountID, store.KeyScope{AllDevices: true, AllMethods: true, AllowTunnel: true})
}

// ForScope returns a DeviceResolver restricted to the devices an API key's scope
// permits. Method and tunnel gating live at their own call sites; here we enforce
// only the device dimension, which both /mcp tools and /connect funnel through.
func (f *Factory) ForScope(accountID string, scope store.KeyScope) mcp.DeviceResolver {
	return &accountResolver{store: f.Store, hub: f.Hub, accountID: accountID, scope: scope}
}

type accountResolver struct {
	store     *store.Store
	hub       *relay.Hub
	accountID string
	scope     store.KeyScope
}

// AccountID is the account this resolver is scoped to. The MCP file-transfer
// tools use it to stage and read blobs on the caller's behalf.
func (a *accountResolver) AccountID() string { return a.accountID }

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
		if !a.scope.AllowsDevice(d.ID) {
			return nil, fmt.Errorf("device %q (%s) is not permitted for this API key — call list_devices to see the devices it can reach", d.Name, d.ID)
		}
		dc, ok := a.hub.Get(d.ID)
		if !ok {
			return nil, fmt.Errorf("device %q (%s) is not connected — open the abacad app on it", d.Name, d.ID)
		}
		dc.SetHumanize(d.Humanize)
		return dc, nil
	}

	// No device_id: pick the most-recently-active device that is online and in scope.
	devices, err := a.store.DevicesByAccount(a.accountID)
	if err != nil {
		return nil, err
	}
	for _, d := range devices { // already ordered last_seen desc
		if !a.scope.AllowsDevice(d.ID) {
			continue
		}
		if dc, ok := a.hub.Get(d.ID); ok {
			dc.SetHumanize(d.Humanize)
			return dc, nil
		}
	}
	// Phrasing kept compatible with the smoke retry on /no device connected/.
	return nil, errors.New("no device connected — open the abacad app on one of your devices and connect it, then try again (see list_devices)")
}

// List returns the account's devices with live status for the list_devices tool.
func (a *accountResolver) List(_ context.Context) ([]mcp.DeviceSummary, error) {
	devices, err := a.store.DevicesByAccount(a.accountID)
	if err != nil {
		return nil, err
	}
	out := make([]mcp.DeviceSummary, 0, len(devices))
	for _, d := range devices {
		if !a.scope.AllowsDevice(d.ID) {
			continue // don't reveal devices this key can't reach
		}
		s := mcp.DeviceSummary{DeviceID: d.ID, Name: d.Name, Online: a.hub.Online(d.ID), Platform: d.Platform, Version: d.Version}
		if d.LastSeen > 0 {
			s.LastSeen = time.Unix(d.LastSeen, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, s)
	}
	return out, nil
}
