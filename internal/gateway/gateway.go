// Package gateway owns the per-device listeners that carry VNC traffic.
//
// There are two kinds of listener:
//
//   - a raw TCP port per device, for agents that bring their own VNC client;
//   - a WebSocket endpoint per device, for noVNC in the browser.
//
// Both end in the same place: a handshake (or a blind pipe in l4 mode) and then
// bytes. The gateway never decodes a framebuffer.
package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"abacad/internal/config"
	"abacad/internal/inventory"
	"abacad/internal/rfb"
)

type Gateway struct {
	inv *inventory.Inventory
	log *slog.Logger

	mu        sync.Mutex
	listeners map[string]*listener // device id -> listener
	limiter   *failLimiter
}

type listener struct {
	id   string
	port int
	ln   net.Listener
}

func New(inv *inventory.Inventory, log *slog.Logger) *Gateway {
	return &Gateway{inv: inv, log: log, listeners: map[string]*listener{}, limiter: newFailLimiter()}
}

// Sync reconciles the running listeners with the current device list. It is
// called at startup and after every add/update/remove.
func (g *Gateway) Sync(devices []inventory.View) {
	desired := map[string]inventory.View{}
	for _, d := range devices {
		if d.VNCPort > 0 && d.Mode != config.ModeDirect {
			desired[d.ID] = d
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	for id, l := range g.listeners {
		d, ok := desired[id]
		if !ok || d.VNCPort != l.port {
			g.log.Info("closing vnc listener", "device", id, "port", l.port)
			_ = l.ln.Close()
			delete(g.listeners, id)
		}
	}

	for id, d := range desired {
		if _, ok := g.listeners[id]; ok {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", d.VNCPort))
		if err != nil {
			g.log.Error("cannot listen for device", "device", id, "port", d.VNCPort, "err", err)
			continue
		}
		g.listeners[id] = &listener{id: id, port: d.VNCPort, ln: ln}
		g.log.Info("vnc listener ready", "device", id, "port", d.VNCPort, "mode", d.Mode)
		go g.accept(ln, id)
	}
}

func (g *Gateway) accept(ln net.Listener, id string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go g.handleTCP(id, conn)
	}
}

func (g *Gateway) handleTCP(id string, conn net.Conn) {
	defer conn.Close()
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	v, ok := g.inv.Device(id)
	if !ok {
		return
	}
	addr := net.JoinHostPort(v.Host, fmt.Sprint(v.Port))
	key := id + "|" + conn.RemoteAddr().String()
	if !g.limiter.allow(key) {
		g.log.Warn("raw vnc rate limited", "device", id, "remote", conn.RemoteAddr().String())
		return
	}
	start := time.Now()
	g.log.Info("raw vnc connect", "device", id, "remote", conn.RemoteAddr().String(), "mode", v.Mode)

	var err error
	ready := false
	markReady := func() {
		ready = true
		g.inv.MarkOnline(id)
	}
	if v.Mode == config.ModeL4 {
		err = pipeL4(conn, addr)
	} else {
		err = rfb.Proxy(context.Background(), conn, rfb.Options{
			UpstreamAddr:   addr,
			UpstreamPass:   v.Password,
			UpstreamUser:   v.Username,
			UpstreamARD:    v.Auth == config.AuthARD,
			DownstreamAuth: rfb.AuthVNC,
			Token:          v.Token,
			Timeout:        5 * time.Second,
			OnReady:        markReady,
		})
	}
	if err != nil {
		switch {
		case rfb.IsDownstreamAuthFailure(err):
			// The hub rejected the client token before touching the device.
			g.inv.SetSessionError(id, err)
		case ready:
			g.inv.SetSessionError(id, err)
		default:
			g.inv.MarkOffline(id, err)
		}
		g.limiter.fail(key)
	} else {
		g.limiter.reset(key)
	}
	g.log.Info("raw vnc close", "device", id, "dur", time.Since(start), "err", err)
}

// HandleWS serves one noVNC WebSocket session. The caller has already checked
// the hub cookie.
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
	g.log.Info("browser vnc connect", "device", id, "remote", c.RemoteAddr().String())

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
	g.log.Info("browser vnc close", "device", id, "dur", time.Since(start), "err", err)
	return err
}

// Close stops every listener. Active sessions keep running until their sockets
// close; a hub shutdown does not need to wait for them.
func (g *Gateway) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id, l := range g.listeners {
		_ = l.ln.Close()
		delete(g.listeners, id)
	}
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
