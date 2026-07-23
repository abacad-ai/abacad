---
title: Transport — control plane vs data plane
description: How bytes move between the abacad server and a device — a JSON control plane over WebSocket and a binary data plane over HTTP (/blobs), split by message type, so large payloads never ride the command socket.
---

How bytes move between the server and a device. One rule decides which of two
channels anything travels on, and it's decided by **type**, not by measuring size at
runtime.

## The rule

> **Binary blobs never travel on the control WebSocket.** Blobs (images, files,
> media, recordings) always go through the **data plane** (HTTP) and are referenced
> by a blob id. The WebSocket is the **control plane**: it carries only structured
> JSON — commands, replies, status, metadata, and blob *references*.

Equivalent test, applied per message type at design time: **if you'd have to base64
it, it's a blob → data plane.** No size threshold, no gray zone, no fallback path.
Keeping large binary payloads off the command socket keeps that socket responsive and
its per-message memory bound small — nothing legitimate on it is ever large. Adding a
feature adds a *message type*, not an endpoint.

## What rides which channel

| Kind | Plane |
|---|---|
| Commands (tap, swipe, input_text, back/home/recents) | WS (control) |
| Replies / acks, correlated by id | WS (control) |
| Status, heartbeat, connect/disconnect events | WS (control) |
| Structured results — dimensions, UI tree, blob refs | WS (control) |
| **Screenshot image bytes** | HTTP (data) |
| **Files, media, recordings** | HTTP (data) |

The UI tree is structured JSON, so it stays inline even though it can reach a few
hundred KB — the type rule answers this without a size check. If the tree ever became
a problem you'd *paginate* it, not blobify it.

## The data plane: one generic blob pair

Not two endpoints per type (screenshot, file, …). **One** type-agnostic pair of dumb
byte pipes. All meaning lives in the control frames that reference a blob id.

```
POST /blobs
  auth:  session cookie, MCP bearer, or device token — any of the server's
         identities, all resolving to the owning account
  body:  raw bytes, streamed straight to disk (never buffered whole)
  hdrs:  Content-Type preserved as blob metadata
  → 201  { id, size, sha256 }

GET /blobs/{id}
  auth:  same; the blob's account must match the caller's (404 otherwise —
         no existence leak across accounts)
  range: supported (resumable download)
  → 200/206  streamed bytes, stored Content-Type
```

- The endpoints **never branch on kind**. A screenshot blob and a 1 GB file blob take
  the same path.
- **Streamed, not buffered** — copied to/from disk on both ends, so a large transfer
  never sits in RAM.
- **NAT direction:** the device dials out, so the server can't push. Both directions
  are **client-initiated**: upload = client `POST`, download = client `GET`. When the
  server wants to deliver a file *to* the phone, it sends a WS control frame and the
  phone does the `GET`.
- **Account-scoped, no cross-account existence leak:** a `GET` for a blob owned by
  another account returns `404`, not `403` — the response never confirms the blob
  exists.
- **Lifecycle is policy, not endpoint shape:** screenshot blobs are throwaway (short
  TTL / delete-after-fetch); file blobs persist. Retention is decided by the control
  layer that created the blob, not by a second endpoint.
- Every blob returns a **sha-256** for integrity; the id itself is opaque to callers.

### Not the same as the `/connect` tunnel

`/blobs` and the `/connect` tunnel are **different lanes for different jobs**, not
competing versions of one thing:

- **`/blobs`** — the app's own discrete objects (a screenshot, a file), addressed by
  id, stored server-side, fetched by whoever holds the id. Application data plane.
- **`/connect`** — a raw TCP tunnel making a device-reachable `host:port` reachable to
  an agent-side client (ssh, rsync, git, a DB). The relay never stores or interprets
  the bytes; it's transport, not storage.

The **[SSH jump host](/docs/guides/ssh/)** is a third consumer of the same device
tunnel: it fronts the tunnel with an SSH server so a stock `ssh` client with no helper
can reach a device's `sshd`. Same blind byte-mover underneath.

## How each feature maps onto it

Every feature is a control-frame choreography over the same `/blobs` pair — a method
name and a bit of choreography, **zero new endpoints**.

**Screenshot** — image → data plane, metadata → control plane:
```
server → WS: { id, method: "screenshot" }
device:      capture → POST /blobs (image/jpeg) → image_id
device → WS: { id, ok, result: { w, h, image_id, tree } }
agent/dash:  GET /blobs/{image_id}
```

**Pull a file off the device:**
```
server → WS: { id, method: "upload_file", params: { path } }
device:      read file → POST /blobs → blob_id
device → WS: { id, ok, result: { blob_id, size } }
```

**Push a file to the device:**
```
caller:      POST /blobs → blob_id
server → WS: { id, method: "download_file", params: { blob_id, dest } }
device:      GET /blobs/{blob_id} → stream to dest
device → WS: { id, ok, result: { written } }
```
