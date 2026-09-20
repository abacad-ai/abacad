package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"abacad/internal/protocol"
	"abacad/internal/relay"
)

const computerGrantTTL = 5 * time.Second

func computerContractTools() []actionTool {
	return []actionTool{
		{
			name:         "observe",
			method:       protocol.MethodObserve,
			description:  "Observe one exact Linux screen state. Screenshot bytes stay on the authenticated /blobs data plane; the result returns state_id, frame_id, blob_id, dimensions, and the capability manifest hash.",
			schema:       `{"type":"object","properties":{` + deviceIDSchema + `,"command_id":{"type":"string"},"correlation_id":{"type":"string"},"idempotency_key":{"type":"string"},"deadline":{"type":"string","format":"date-time"}},"required":["command_id","correlation_id","idempotency_key","deadline"],"additionalProperties":false}`,
			computerCall: observeComputer,
		},
		{
			name:         "act",
			method:       protocol.MethodAct,
			description:  "Execute exactly one Linux X11 left click bound to an observed state. Requires a short-lived grant, manifest hash, exact target state/frame, command identities, and an absolute deadline. Unknown outcomes must be reconciled; this tool never blindly replays a click.",
			schema:       `{"type":"object","properties":{` + deviceIDSchema + `,"command_id":{"type":"string"},"correlation_id":{"type":"string"},"idempotency_key":{"type":"string"},"deadline":{"type":"string","format":"date-time"},"capability_manifest_hash":{"type":"string"},"state_id":{"type":"string"},"frame_id":{"type":"string"},"x":{"type":"integer"},"y":{"type":"integer"}},"required":["command_id","correlation_id","idempotency_key","deadline","capability_manifest_hash","state_id","frame_id","x","y"],"additionalProperties":false}`,
			computerCall: actComputer,
		},
		{
			name:         "reconcile",
			method:       protocol.MethodReconcile,
			description:  "Read the durable receipt for a computer command after a lost reply. Reconciliation is read-only and never replays an unknown click.",
			schema:       `{"type":"object","properties":{` + deviceIDSchema + `,"command_id":{"type":"string"},"correlation_id":{"type":"string"},"idempotency_key":{"type":"string"},"deadline":{"type":"string","format":"date-time"}},"required":["command_id","correlation_id","idempotency_key","deadline"],"additionalProperties":false}`,
			computerCall: reconcileComputer,
		},
	}
}

type computerArgs struct {
	DeviceID               string `json:"device_id"`
	CommandID              string `json:"command_id"`
	CorrelationID          string `json:"correlation_id"`
	IdempotencyKey         string `json:"idempotency_key"`
	Deadline               string `json:"deadline"`
	CapabilityManifestHash string `json:"capability_manifest_hash"`
	StateID                string `json:"state_id"`
	FrameID                string `json:"frame_id"`
	X                      int    `json:"x"`
	Y                      int    `json:"y"`
}

func parseComputerArgs(raw json.RawMessage, action bool) (computerArgs, time.Time, error) {
	var a computerArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return a, time.Time{}, fmt.Errorf("invalid computer args: %w", err)
	}
	if a.CommandID == "" || a.CorrelationID == "" || a.IdempotencyKey == "" || a.Deadline == "" {
		return a, time.Time{}, fmt.Errorf("computer command_id, correlation_id, idempotency_key, and deadline are required")
	}
	deadline, err := time.Parse(time.RFC3339Nano, a.Deadline)
	if err != nil || !deadline.After(time.Now()) {
		return a, time.Time{}, fmt.Errorf("deadline must be a future RFC3339 timestamp")
	}
	if action && (a.CapabilityManifestHash == "" || a.StateID == "" || a.FrameID == "") {
		return a, time.Time{}, fmt.Errorf("act requires capability_manifest_hash, state_id, and frame_id")
	}
	return a, deadline, nil
}

func computerText(r protocol.ComputerResult, err error) toolResult {
	b, marshalErr := json.MarshalIndent(r, "", "  ")
	if marshalErr != nil {
		return errorResult(marshalErr.Error())
	}
	if err != nil {
		return toolResult{Content: []content{textContent(string(b)), textContent("Error: " + err.Error())}, IsError: true}
	}
	return textResult(string(b))
}

func observeComputer(ctx context.Context, dc *relay.DeviceConn, raw json.RawMessage, _ string, _ ComputerGrantIssuer) toolResult {
	a, deadline, err := parseComputerArgs(raw, false)
	if err != nil {
		return errorResult(err.Error())
	}
	r, sendErr := dc.SendComputer(ctx, protocol.ComputerCommand{
		Contract: protocol.ContractID, Method: protocol.ComputerObserve,
		CommandID: a.CommandID, CorrelationID: a.CorrelationID,
		IdempotencyKey: a.IdempotencyKey, Deadline: deadline,
	})
	return computerText(r, sendErr)
}

func actComputer(ctx context.Context, dc *relay.DeviceConn, raw json.RawMessage, accountID string, grants ComputerGrantIssuer) toolResult {
	a, deadline, err := parseComputerArgs(raw, true)
	if err != nil {
		return errorResult(err.Error())
	}
	if grants == nil {
		return errorResult("computer grants are not configured")
	}
	grant, err := grants.Issue(accountID, dc.DeviceID, "act:left_click", computerGrantTTL)
	if err != nil {
		return errorResult(err.Error())
	}
	if err := grants.Validate(accountID, dc.DeviceID, grant, "act:left_click", time.Now()); err != nil {
		return errorResult(err.Error())
	}
	r, sendErr := dc.SendComputer(ctx, protocol.ComputerCommand{
		Contract: protocol.ContractID, Method: protocol.ComputerAct,
		CommandID: a.CommandID, CorrelationID: a.CorrelationID,
		IdempotencyKey: a.IdempotencyKey, Deadline: deadline,
		CapabilityManifestHash: a.CapabilityManifestHash,
		Grant:                  &grant,
		Target:                 &protocol.ComputerTarget{StateID: a.StateID, FrameID: a.FrameID},
		Action:                 &protocol.LeftClick{Kind: "left_click", X: a.X, Y: a.Y},
	})
	return computerText(r, sendErr)
}

func reconcileComputer(ctx context.Context, dc *relay.DeviceConn, raw json.RawMessage, _ string, _ ComputerGrantIssuer) toolResult {
	a, deadline, err := parseComputerArgs(raw, false)
	if err != nil {
		return errorResult(err.Error())
	}
	r, sendErr := dc.SendComputer(ctx, protocol.ComputerCommand{
		Contract: protocol.ContractID, Method: protocol.ComputerReconcile,
		CommandID: a.CommandID, CorrelationID: a.CorrelationID,
		IdempotencyKey: a.IdempotencyKey, Deadline: deadline,
	})
	return computerText(r, sendErr)
}
