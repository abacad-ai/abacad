// Package sshjump is the SSH "jump host" that makes every device reachable as
// <device>.<base-domain> for a stock `ssh -J` / ProxyJump client — with nothing
// to install on the client and no ProxyCommand.
//
// How it works: a client does `ssh <device>.abacad.ai` with
//
//	Host *.abacad.ai
//	    ProxyJump abacad.ai
//
// which is `ssh -W <device>.abacad.ai:22 abacad.ai` under the hood. OpenSSH
// connects to the jump (abacad.ai), authenticates with the user's key, and asks
// the jump to open a direct-tcpip channel to "<device>.abacad.ai:22" — and that
// target host string travels IN-BAND (RFC 4254 §7.2), which is the hook a bare
// SSH connection lacks (no SNI/Host header). We read the device label from it,
// map it to a device, and bridge the channel into that device's tunnel to its
// own 127.0.0.1:22.
//
// The inner SSH session (client <-> device sshd) is opaque to us — we move
// ciphertext, hold no session keys, and never see the login. Our auth is only
// *authorization*: the client's public key identifies the account, and we route
// solely to devices that account owns.
package sshjump

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Server is the SSH jump host. Zero auth of the inner session; it authorizes the
// account by public key and routes direct-tcpip channels to owned devices.
type Server struct {
	Addr       string     // listen address, e.g. ":22"
	BaseDomain string     // e.g. "abacad.ai"; the suffix stripped to get the device label
	HostSigner ssh.Signer // the jump's host key (users pin it in known_hosts)

	// AccountForKey maps an offered public key to the owning account id. Returning
	// an error rejects the connection (unknown key).
	AccountForKey func(key ssh.PublicKey) (accountID string, err error)

	// OpenTunnel bridges to the device's local sshd. It must enforce that the
	// account owns the device and that it is online, then open a stream to the
	// device's 127.0.0.1:22. A non-nil error rejects the channel.
	OpenTunnel func(ctx context.Context, accountID, deviceID string) (io.ReadWriteCloser, error)
}

// ListenAndServe binds Addr and serves until the listener errors. It blocks.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	log.Printf("ssh jump host     : ssh <device>.%s  (listening on %s)", s.BaseDomain, s.Addr)
	return s.Serve(ln)
}

// Serve accepts connections on ln until it errors. It blocks.
func (s *Server) Serve(ln net.Listener) error {
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			accountID, err := s.AccountForKey(key)
			if err != nil {
				return nil, errors.New("unauthorized key — add it to your Abacad account")
			}
			// Carry the account id to the channel handler via the connection's
			// permissions (the only per-connection state ssh exposes to us).
			return &ssh.Permissions{Extensions: map[string]string{"account_id": accountID}}, nil
		},
	}
	cfg.AddHostKey(s.HostSigner)

	for {
		nConn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(nConn, cfg)
	}
}

func (s *Server) handleConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	sshConn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		// Auth failure or handshake error; NewServerConn already closed nConn.
		return
	}
	defer sshConn.Close()
	accountID := sshConn.Permissions.Extensions["account_id"]

	go ssh.DiscardRequests(reqs) // no global requests are meaningful to a jump
	for newChan := range chans {
		// A jump host only serves direct-tcpip (port forwarding). Reject shells,
		// exec, sftp, etc. — there is nothing to log into here.
		if newChan.ChannelType() != "direct-tcpip" {
			_ = newChan.Reject(ssh.Prohibited, "this is a jump host: only direct-tcpip is served")
			continue
		}
		go s.handleForward(accountID, newChan)
	}
}

// directTCPIP is the RFC 4254 §7.2 direct-tcpip channel-open payload.
type directTCPIP struct {
	HostToConnect  string
	PortToConnect  uint32
	OriginatorIP   string
	OriginatorPort uint32
}

func (s *Server) handleForward(accountID string, newChan ssh.NewChannel) {
	var req directTCPIP
	if err := ssh.Unmarshal(newChan.ExtraData(), &req); err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, "malformed direct-tcpip request")
		return
	}
	deviceID, err := DeviceIDFromHost(req.HostToConnect, s.BaseDomain)
	if err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}

	// The target port is pinned to the device's own sshd. We ignore the client's
	// requested port so the jump can never be used to reach arbitrary internal
	// ports on the device's host — it forwards to SSH and nothing else.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnel, err := s.OpenTunnel(ctx, accountID, deviceID)
	if err != nil {
		_ = newChan.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer tunnel.Close()

	ch, chReqs, err := newChan.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	go ssh.DiscardRequests(chReqs)

	// Bridge both directions; whichever end closes first tears down the other.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(tunnel, ch); done <- struct{}{} }() // client -> device
	go func() { _, _ = io.Copy(ch, tunnel); done <- struct{}{} }() // device -> client
	<-done
}

// DeviceIDFromHost extracts a device id from a forwarding target host. The label
// is the device id with '_' rendered as '-' (device ids are dev_<base32>, and
// '_' is not a valid DNS label character): "dev-ab3x.abacad.ai" -> "dev_ab3x".
func DeviceIDFromHost(host, baseDomain string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	suffix := "." + strings.ToLower(baseDomain)
	if !strings.HasSuffix(host, suffix) {
		return "", fmt.Errorf("host %q is not under %s", host, baseDomain)
	}
	label := strings.TrimSuffix(host, suffix)
	if label == "" || strings.Contains(label, ".") {
		return "", fmt.Errorf("bad device label %q", label)
	}
	return strings.ReplaceAll(label, "-", "_"), nil
}

// HostForDevice is the inverse: the ssh hostname a client uses for a device.
func HostForDevice(deviceID, baseDomain string) string {
	return strings.ReplaceAll(deviceID, "_", "-") + "." + baseDomain
}

// LoadOrCreateHostKey returns the jump's host signer, generating and persisting a
// new ed25519 key (0600) at path on first use so known_hosts stays stable across
// restarts.
func LoadOrCreateHostKey(path string) (ssh.Signer, error) {
	if b, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(b)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	block, err := ssh.MarshalPrivateKey(priv, "abacad-jump")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return nil, err
	}
	return ssh.NewSignerFromSigner(priv)
}
