# Transport: control plane vs data plane

How bytes move between the server and a device. One rule decides which of the two
channels anything travels on, and it's decided by **type**, never by measuring size at
runtime.

---

## The rule

> **Binary blobs never travel on the WebSocket.** Blobs (images, files, media,
> recordings) always go through the **data plane** (HTTP) and are referenced by a
> blob id. The WebSocket is the **control plane**: it carries only structured JSON —
> commands, replies, status, metadata, and blob *references*.

Equivalent test, applied per message type at design time: **if you'd have to base64 it,
it's a blob → data plane.** No size threshold, no gray zone, no fallback path. Adding a
feature adds a *message type*, not an endpoint.

### Why (the incident that forced it)

Screenshots were sent inline as base64 over the WebSocket. The device's okhttp client has
a fixed **16 MiB outbound queue** (`RealWebSocket.MAX_QUEUE_SIZE`, not configurable); when
an agent screenshot (full-res, lossless PNG + UI tree) overlapped a dashboard screenshot,
the two replies overran the queue and okhttp dropped the socket with `close(1001)` — the
device flapped once per capture. base64-on-a-text-frame was the smell; the type rule
removes the whole bug class.

---

## What rides which channel

| Kind | Plane | Always |
|---|---|---|
| Commands (tap, swipe, input_text, back/home/recents) | WS (control) | ✓ |
| Replies / acks, correlated by id | WS (control) | ✓ |
| Status, heartbeat, connect/disconnect events | WS (control) | ✓ |
| Structured results — dimensions, UI tree, blob refs | WS (control) | ✓ |
| **Screenshot image bytes** | HTTP (data) | ✓ |
| **Files, media, recordings** | HTTP (data) | ✓ |

The UI tree is structured JSON, so it stays inline even though it can reach a few hundred
KB — the type rule answers this without a size check. If the tree ever became a problem
you'd *paginate* it, not blobify it.

---

## The data plane: one generic blob pair

Not two endpoints per type (screenshot, file, …). **One** type-agnostic pair of dumb byte
pipes. All meaning lives in the control frames that reference a blob id.

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

- The endpoints **never branch on kind**. A screenshot blob and a 1 GB file blob take the
  same path.
- **Streamed, not buffered** — `io.Copy` to/from disk on both ends, so a large transfer
  never sits in RAM and no single frame ever threatens okhttp's 16 MiB cap.
- **NAT direction:** the device dials out, so the server can't push. Both directions are
  **client-initiated**: upload = client `POST`, download = client `GET`. When the server
  wants to deliver a file *to* the phone, it sends a WS control frame and the phone does
  the `GET`.
- **Lifecycle is policy, not endpoint shape:** screenshot blobs are throwaway (short TTL /
  delete-after-fetch); file blobs persist. Retention is decided by the control layer that
  created the blob, not by a second endpoint.
- Blob id may be an opaque server id or a content hash (sha-256 → integrity + dedup);
  either way it's opaque to callers. (Today: opaque `blob_<random>` id, with the sha-256
  computed and returned for integrity — dedup deferred.)

### Not the same as the `/connect` tunnel

`/blobs` (this doc) and the `/connect` tunnel are **different lanes for different jobs**,
not competing versions of one thing:

- **`/blobs`** — the app's own discrete objects (a screenshot, a file), addressed by id,
  stored server-side, fetched by whoever holds the id. Application data plane.
- **`/connect`** — a raw TCP tunnel making a device-reachable `host:port` reachable to an
  agent-side client (ssh, rsync, git, a DB). The relay never stores or interprets the
  bytes; it's transport, not storage.

You could move a file *through* the tunnel with `rsync`, but that's reaching a host, not
storing an app object. Screenshots and app files use `/blobs`; reaching services uses
`/connect`.

---

## How each feature maps onto it

Every feature is a control-frame choreography over the same `/blobs` pair.

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

A new blob-bearing feature adds a method name and a bit of choreography — **zero new
endpoints.**

---

## Status

- **Adopted** as the target design.
- **`/blobs` is implemented** (`internal/blob`): `POST /blobs` + `GET /blobs/{id}`, streamed
  to/from disk under the `-blobs` dir, metadata in the `blobs` table, per-account auth across
  session / MCP / device-token identities, `-max-blob-bytes` cap (default 1 GiB), Range on
  download. Round-trip verified (upload via MCP token, download via device token, bytes +
  sha-256 match). Lifecycle/GC and content-dedup are deferred.
- **Screenshots today still send inline JPEG over the WS** (a one-line stopgap that stops
  the `close(1001)` flapping immediately: `CompressFormat.JPEG, 85` instead of lossless
  PNG). This is *not* the destination — the type rule moves the image onto the data plane
  (`image_id` reference via `/blobs`) as above, which is what permanently ends the
  queue-overrun class. That migration is the next step, now that `/blobs` exists.
- `readLimit` on the device socket stays **small (16 MiB)** on purpose: it's a per-message
  memory bound, and under this design nothing legitimate on the WS is ever large.
