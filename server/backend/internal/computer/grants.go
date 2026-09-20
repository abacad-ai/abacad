// Package computer owns the server-side short-lived grants for the fixed
// abacad.computer action slice. Grants are deliberately narrow, in-memory
// capability records: the durable action receipt lives on the device journal.
package computer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"abacad/internal/protocol"
)

const LeftClickScope = "act:left_click"

var (
	ErrGrantNotFound = errors.New("computer grant not found")
	ErrGrantRevoked  = errors.New("computer grant revoked")
	ErrGrantExpired  = errors.New("computer grant expired")
	ErrGrantScope    = errors.New("computer grant scope denied")
)

type grantRecord struct {
	grant   protocol.ComputerGrant
	account string
	revoked bool
}

// Grants is a process-local issuer and revocation registry. Every action is
// validated immediately before relay; a new process starts with no outstanding
// grants, which is the safe failure mode.
type Grants struct {
	mu sync.Mutex
	m  map[string]grantRecord
}

func NewGrants() *Grants { return &Grants{m: make(map[string]grantRecord)} }

func (g *Grants) Issue(accountID, deviceID, scope string, ttl time.Duration) (protocol.ComputerGrant, error) {
	if g == nil {
		return protocol.ComputerGrant{}, errors.New("computer grants unavailable")
	}
	if accountID == "" || deviceID == "" {
		return protocol.ComputerGrant{}, errors.New("computer grant requires account and device")
	}
	if scope != LeftClickScope {
		return protocol.ComputerGrant{}, fmt.Errorf("unsupported computer scope %q", scope)
	}
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	now := time.Now().UTC()
	id, err := randomID("grant")
	if err != nil {
		return protocol.ComputerGrant{}, err
	}
	grant := protocol.ComputerGrant{
		GrantID: id, DeviceID: deviceID, Scope: scope,
		IssuedAt: now, ExpiresAt: now.Add(ttl),
	}
	g.mu.Lock()
	g.m[id] = grantRecord{grant: grant, account: accountID}
	g.mu.Unlock()
	return grant, nil
}

func (g *Grants) Revoke(grantID string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	if r, ok := g.m[grantID]; ok {
		r.revoked = true
		g.m[grantID] = r
	}
	g.mu.Unlock()
}

func (g *Grants) Validate(accountID, deviceID string, grant protocol.ComputerGrant, scope string, now time.Time) error {
	if g == nil {
		return errors.New("computer grants unavailable")
	}
	g.mu.Lock()
	r, ok := g.m[grant.GrantID]
	g.mu.Unlock()
	if !ok || r.account != accountID || r.grant.DeviceID != deviceID {
		return ErrGrantNotFound
	}
	if r.revoked {
		return ErrGrantRevoked
	}
	if scope != LeftClickScope || grant.Scope != scope || r.grant.Scope != scope {
		return ErrGrantScope
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !now.Before(r.grant.ExpiresAt) || !now.Before(grant.ExpiresAt) {
		return ErrGrantExpired
	}
	if grant.IssuedAt.IsZero() || grant.ExpiresAt.IsZero() || grant.IssuedAt.Before(r.grant.IssuedAt) || grant.ExpiresAt.After(r.grant.ExpiresAt) {
		return ErrGrantExpired
	}
	return nil
}

func randomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b), nil
}
