# `abacad.computer`

This page is the authoritative contract for computer use in Abacad. The contract is
fixed and unversioned: every caller uses the literal contract id `abacad.computer`.
The implementation is an agent-agnostic execution substrate. It does not choose a
model, interpret a task, approve meaning, judge prompt injection, run an agent loop,
or retain a task trajectory.

## Responsibility boundary

Outside Abacad, the calling agent or harness owns:

- model and tool policy;
- task interpretation, semantic approval, consent, and prompt-injection judgment;
- `observe → decide → act` orchestration;
- context, session, checkpoint, trajectory, and task replay.

Inside Abacad, the tool owns only reliable observation, constrained input execution,
post-action identity, reconciliation, capability and grant enforcement, and a minimal
receipt/audit record.

## Transport

The control plane is authenticated JSON over the existing device WebSocket. It carries
commands, replies, identifiers, state metadata, capabilities, grants, and receipt
metadata. Screenshot bytes never travel in a control frame.

The data plane is authenticated HTTP at `/blobs`. A device uploads a screenshot with
`POST /blobs`, receives an opaque `blob_id`, and returns that reference in its control
reply. A caller downloads bytes with `GET /blobs/{blob_id}` using its own authenticated
identity. Blob storage is account- and device-scoped, streamed, immutable, and subject
to the configured retention policy. Audit and control replies contain metadata and
references only; they do not contain screenshot bytes or bearer credentials.

## Capabilities and grants

A device declares a canonical capability manifest. For this slice the manifest is the
sorted set:

```json
["act:left_click", "observe", "reconcile"]
```

The manifest hash is SHA-256 over its canonical JSON UTF-8 bytes. The hash is returned
with every observation and is bound into every action request. A caller must not use a
manifest hash it did not observe.

The server issues a short-lived grant for one device and one scope. A grant contains
an opaque `grant_id`, `device_id`, `scope`, `issued_at`, `expires_at`, and `revoked`
state held by the server. The server validates the grant immediately before sending a
command; an expired or revoked grant is rejected without contacting the device. The
Linux device independently rejects an expired grant, a grant for another device, a
scope other than `act:left_click`, or a command whose manifest hash does not match its
local manifest. Revocation is checked on every action and is fail-closed.

## `observe`

Request:

```json
{
  "contract": "abacad.computer",
  "method": "observe",
  "command_id": "cmd_…",
  "correlation_id": "corr_…",
  "idempotency_key": "idem_…",
  "deadline": "2026-08-18T12:00:00.000000000Z",
  "capability_manifest_hash": "sha256:…"
}
```

The device captures one screen atomically with its state identity, uploads the JPEG
bytes through `/blobs`, and replies:

```json
{
  "contract": "abacad.computer",
  "command_id": "cmd_…",
  "correlation_id": "corr_…",
  "status": "succeeded",
  "effect": "confirmed",
  "state_id": "state_…",
  "frame_id": "frame_<sha256>",
  "blob_id": "blob_…",
  "width": 1280,
  "height": 800,
  "capability_manifest_hash": "sha256:…",
  "receipt": {
    "command_id": "cmd_…",
    "status": "succeeded",
    "effect": "confirmed",
    "state_id": "state_…",
    "frame_id": "frame_<sha256>"
  }
}
```

`state_id` and `frame_id` are exact identities from the same capture. A frame id is
the SHA-256 identity of the uploaded JPEG bytes. A state id changes for each committed
capture and is never reused for a different frame. The caller downloads `blob_id`
only after it receives the control reply.

## `act` — one left click

The first complete action is deliberately one operation: a single left click at an
absolute X11 root-screen coordinate. No other action is part of this contract slice.

Request:

```json
{
  "contract": "abacad.computer",
  "method": "act",
  "command_id": "cmd_…",
  "correlation_id": "corr_…",
  "idempotency_key": "idem_…",
  "deadline": "2026-08-18T12:00:00.000000000Z",
  "capability_manifest_hash": "sha256:…",
  "grant": {
    "grant_id": "grant_…",
    "scope": "act:left_click",
    "issued_at": "2026-08-18T11:59:59.000000000Z",
    "expires_at": "2026-08-18T12:00:05.000000000Z"
  },
  "target": {
    "state_id": "state_…",
    "frame_id": "frame_<sha256>"
  },
  "action": {"kind": "left_click", "x": 640, "y": 400}
}
```

The device validates the deadline, grant, scope, manifest hash, and target identity
before injection. It records the command in its durable journal before touching X11,
executes exactly one left-click, captures an after-state, uploads the after JPEG, then
commits a minimal receipt. Reusing an idempotency key returns the original receipt and
does not inject another click. An already-seen command is never replayed blindly.

## `reconcile`

Reconciliation is read-only and is the only recovery operation after a lost reply:

```json
{
  "contract": "abacad.computer",
  "method": "reconcile",
  "command_id": "cmd_…",
  "correlation_id": "corr_…",
  "idempotency_key": "reconcile-…",
  "deadline": "2026-08-18T12:00:10.000000000Z"
}
```

The device looks up the command or idempotency key in its journal and returns the
recorded receipt. It does not execute an action during reconciliation. If the journal
contains a pre-execution record but no committed effect, the reply is
`unknown_outcome` with `effect: "unknown"`; the caller must observe again and make a
new semantic decision. Reconciliation never retries a click.

## Outcome semantics

Every `observe`, `act`, and `reconcile` reply has one of these statuses and effects:

| `status` | `effect` | Meaning |
|---|---|---|
| `succeeded` | `confirmed` | The requested observation or click completed and its resulting identity is known. |
| `rejected` | `none` | Validation, capability, grant, stale target, or deadline checks stopped execution before X11. |
| `timeout` | `none` | The device did not begin the operation before the absolute deadline. |
| `unknown_outcome` | `unknown` | Execution may have started, but a final receipt was not durably observed. Reconcile; do not replay. |

A receipt contains only command/correlation/idempotency identifiers, status, effect,
state/frame identities, capability hash, and timestamps. It never contains screenshot
bytes, grant secrets, or arbitrary input text. The device journal is append-only and
fsync-backed; the server activity trail records the command outcome and actor without
capturing image bytes.

## Linux slice

The verified slice is:

1. Linux X11 captures a JPEG and uploads it to `/blobs`.
2. `observe` returns exact `state_id` / `frame_id` plus the blob reference.
3. A caller binds one left click to that observation and a short-lived grant.
4. Linux validates the request, journals it, injects one XTEST left-click, captures and
   uploads the after frame, and returns a succeeded receipt.
5. Disconnect or reply loss is recovered with read-only `reconcile`; no blind replay.

The slice intentionally does not define shell execution, browser scripting, gestures,
keyboard input, multi-pointer input, Wayland, or task orchestration.

## Provenance

The accepted design source is `computer-use-vs-abacad/design` with SHA-256
`e3e133c5dd25dbec9ecd0aface1aa3ef5b0058755b228170cd0daa0ca0ff254e`; its geometry has
SHA-256 `52e9e21f7235af0bd85000fcfc2cdb8e5fb0b9d595e21b7d8195aee27335d281`.
The research artifact `computer-use-vs-abacad/review` remains a separate, untouched
reference.
