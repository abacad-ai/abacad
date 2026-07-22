package api

import (
	"net/http"

	"abacad/internal/relay"
)

// vncStart opens a live VNC session for a device the caller owns and returns the
// browser viewer ticket + the noVNC watch path. The device is told (over the
// command hub) to start its VNC server and reverse-connect to the ingress; that
// "vnc" command is itself recorded in the activity trail by the device handler's
// command observer, so the takeover boundary is audited.
func (a *API) vncStart(w http.ResponseWriter, r *http.Request) {
	if a.VNC == nil {
		writeErr(w, http.StatusNotImplemented, "live view is not configured")
		return
	}
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	ctx := relay.WithSource(r.Context(), "dashboard")
	ticket, expiresAt, err := a.VNC.Start(ctx, d.ID, account(r).ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     ticket,
		"watch_path": "/vnc/watch?ticket=" + ticket,
		"expires_at": expiresAt,
	})
}

// vncStop ends any live session for a device the caller owns.
func (a *API) vncStop(w http.ResponseWriter, r *http.Request) {
	if a.VNC == nil {
		writeErr(w, http.StatusNotImplemented, "live view is not configured")
		return
	}
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	a.VNC.Stop(d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

// vncStatus reports the device's live session (active / viewer_connected / expiry).
func (a *API) vncStatus(w http.ResponseWriter, r *http.Request) {
	d, err := a.Store.DeviceOwnedBy(r.PathValue("id"), account(r).ID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "device not found")
		return
	}
	if a.VNC == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, a.VNC.Status(d.ID))
}
