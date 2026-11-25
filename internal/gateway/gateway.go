// Package gateway carries VNC traffic between a WebSocket client and a device.
//
// There is one way in: a WebSocket per device at /ws/vnc/{id}, the websockify
// convention that noVNC, py-wsvnc and a local websocat bridge all speak. The
// web layer has already checked the hub key before a session reaches here.
// Each session ends in a handshake (or a blind pipe in l4 mode) and then bytes.
// The gateway never decodes a framebuffer.
package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"abacad/internal/config"
	"abacad/internal/inventory"
	"abacad/internal/rfb"
)

type Gateway struct {
	inv *inventory.Inventory
	log *slog.Logger
}

func New(inv *inventory.Inventory, log *slog.Logger) *Gateway {
	return &Gateway{inv: inv, log: log}
}

// HandleWS serves one WebSocket VNC session. The caller has already checked
// the hub key (cookie, Bearer header or ?token=).
func (g *Gateway) HandleWS(ctx context.Context, c *WSConn, id string) error {
	v, ok := g.inv.Device(id)
	if !ok {
		return fmt.Errorf("device %q not found", id)
	}
	if v.Mode == config.ModeDirect {
		return fmt.Errorf("device %q is catalog-only", id)
	}
	addr := net.JoinHostPort(v.Host, fmt.Sprint(v.Port))
	start := time.Now()
	g.log.Info("vnc connect", "device", id, "remote", c.RemoteAddr().String())

	var err error
	ready := false
	markReady := func() {
		ready = true
		g.inv.MarkOnline(id)
	}
	if v.Mode == config.ModeL4 {
		err = pipeL4(c, addr)
	} else {
		err = rfb.Proxy(ctx, c, rfb.Options{
			UpstreamAddr:   addr,
			UpstreamPass:   v.Password,
			UpstreamUser:   v.Username,
			UpstreamARD:    v.Auth == config.AuthARD,
			DownstreamAuth: rfb.AuthNone,
			Timeout:        5 * time.Second,
			OnReady:        markReady,
		})
	}
	if err != nil {
		if ready {
			g.inv.SetSessionError(id, err)
		} else {
			g.inv.MarkOffline(id, err)
		}
	}
	g.log.Info("vnc close", "device", id, "dur", time.Since(start), "err", err)
	return err
}

func pipeL4(down net.Conn, addr string) error {
	up, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer up.Close()
	if tc, ok := up.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	if tc, ok := down.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(up, down)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(down, up)
		done <- struct{}{}
	}()
	<-done
	_ = up.Close()
	_ = down.Close()
	return nil
}
