package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ContractID is the single computer-use wire contract. It is deliberately a
// literal: callers do not negotiate a contract family on the control plane.
const ContractID = "abacad.computer"

const (
	ComputerObserve   = "observe"
	ComputerAct       = "act"
	ComputerReconcile = "reconcile"
)

const (
	ComputerSucceeded     = "succeeded"
	ComputerRejected      = "rejected"
	ComputerTimeout       = "timeout"
	ComputerUnknown       = "unknown_outcome"
	ComputerConfirmed     = "confirmed"
	ComputerNoEffect      = "none"
	ComputerUnknownEffect = "unknown"
)

// ComputerCapabilityManifest is the canonical capability declaration for the
// first complete slice. The list is sorted before hashing and transmission.
type ComputerCapabilityManifest []string

func (m ComputerCapabilityManifest) Canonical() []string {
	out := append([]string(nil), m...)
	sort.Strings(out)
	return out
}

func (m ComputerCapabilityManifest) Hash() string {
	b, _ := json.Marshal(m.Canonical())
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var LinuxComputerManifest = ComputerCapabilityManifest{"act:left_click", "observe", "reconcile"}

// ComputerGrant is a short-lived, server-issued action scope. Revocation is
// checked by the issuer before a command is sent; the device checks expiry and
// scope independently.
type ComputerGrant struct {
	GrantID   string    `json:"grant_id"`
	DeviceID  string    `json:"device_id"`
	Scope     string    `json:"scope"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ComputerTarget binds an action to the exact observation it was planned from.
type ComputerTarget struct {
	StateID string `json:"state_id"`
	FrameID string `json:"frame_id"`
}

// LeftClick is the only action in the initial Linux slice.
type LeftClick struct {
	Kind string `json:"kind"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
}

// ComputerCommand is the JSON control-plane envelope. ID is the transport
// correlation key; the remaining identifiers are durable caller identities.
type ComputerCommand struct {
	ID                     string          `json:"id"`
	Contract               string          `json:"contract"`
	Method                 string          `json:"method"`
	CommandID              string          `json:"command_id"`
	CorrelationID          string          `json:"correlation_id"`
	IdempotencyKey         string          `json:"idempotency_key"`
	Deadline               time.Time       `json:"deadline"`
	CapabilityManifestHash string          `json:"capability_manifest_hash,omitempty"`
	Grant                  *ComputerGrant  `json:"grant,omitempty"`
	Target                 *ComputerTarget `json:"target,omitempty"`
	Action                 *LeftClick      `json:"action,omitempty"`
}

// ComputerReceipt is the minimal durable result. It contains no screenshot
// bytes, credentials, or arbitrary caller data.
type ComputerReceipt struct {
	CommandID              string    `json:"command_id"`
	CorrelationID          string    `json:"correlation_id,omitempty"`
	IdempotencyKey         string    `json:"idempotency_key,omitempty"`
	Status                 string    `json:"status"`
	Effect                 string    `json:"effect"`
	StateID                string    `json:"state_id,omitempty"`
	FrameID                string    `json:"frame_id,omitempty"`
	CapabilityManifestHash string    `json:"capability_manifest_hash,omitempty"`
	Reason                 string    `json:"reason,omitempty"`
	At                     time.Time `json:"at"`
}

// ComputerResult is returned inside the ordinary protocol Reply.result field.
// Observe and act include an after-frame reference; reconcile may return only a
// receipt when no new capture was taken.
type ComputerResult struct {
	Contract               string           `json:"contract"`
	CommandID              string           `json:"command_id"`
	CorrelationID          string           `json:"correlation_id"`
	IdempotencyKey         string           `json:"idempotency_key"`
	Status                 string           `json:"status"`
	Effect                 string           `json:"effect"`
	StateID                string           `json:"state_id,omitempty"`
	FrameID                string           `json:"frame_id,omitempty"`
	BlobID                 string           `json:"blob_id,omitempty"`
	Width                  int              `json:"width,omitempty"`
	Height                 int              `json:"height,omitempty"`
	CapabilityManifestHash string           `json:"capability_manifest_hash,omitempty"`
	Receipt                *ComputerReceipt `json:"receipt,omitempty"`
	Reason                 string           `json:"reason,omitempty"`
}

func (r ComputerResult) Validate() error {
	if r.Contract != ContractID {
		return fmt.Errorf("unexpected computer contract %q", r.Contract)
	}
	if r.CommandID == "" {
		return fmt.Errorf("computer result missing command_id")
	}
	if r.Status != ComputerSucceeded && r.Status != ComputerRejected && r.Status != ComputerTimeout && r.Status != ComputerUnknown {
		return fmt.Errorf("unknown computer status %q", r.Status)
	}
	return nil
}
