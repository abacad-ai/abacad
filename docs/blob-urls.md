# Signed-URL file transfer — `send_file` / `get_file`

The file-transfer MCP tools deal only in **URLs**, never bytes. The agent calls a
tool, gets a short-lived signed URL, and moves the bytes over plain HTTP — so a
file never crosses the model's context window. This replaces the old
`push_file` / `pull_file` tools, which took bytes inline as tool arguments /
results. See `transport.md` for the underlying `/blobs` data plane.

## The two tools

### `get_file(device_id, path)` — device → agent

1. The server tells the device to upload the file (the unchanged `pull_file`
   device command); the device streams it to `/blobs`.
2. The tool returns a signed **download** URL plus size + sha256.
3. The agent `GET`s the URL to fetch the bytes (Range/resume supported). No bearer
   needed — the signature *is* the authorization. Nothing is inlined.

### `send_file(device_id, path, mode?)` — agent → device

1. The tool returns a signed **upload** URL bound to `(account, device, path,
   mode)`. (The device is resolved + liveness-checked when the URL is minted, so a
   URL is only issued for a device that is currently online.)
2. The agent `POST`s the file bytes to that URL (`POST /blobs/send`). The server
   stores them, delivers them to the device (the unchanged `push_file` device
   command), waits for the device's sha256, and returns the outcome **in the POST
   response** — so the agent learns pass/fail from the POST itself, not a later
   call.

`POST /blobs/send` status codes:

| Code | Meaning |
|---|---|
| `200 {written,size,sha256,path}` | written and device-confirmed |
| `400` | bad, tampered, or expired signature |
| `413` | body exceeds the blob size cap |
| `502` | device write failed, or its sha256 ≠ what we staged |
| `504` | device offline / didn't answer |

The write is idempotent (same path, same bytes → same end state; the device does
temp-write→rename), so a retry after an ambiguous disconnect is safe.

## Signed URLs

Stateless HMAC — no DB row. `sig = hex(HMAC-SHA256(key, canonical))`, verified with
`hmac.Equal`; the signature is checked before the expiry is trusted.

- download canonical: `v1\ndownload\n{blobID}\n{exp}`
- upload canonical:   `v1\nupload\n{acct}\n{device}\n{path}\n{mode}\n{exp}`

Query carries `exp` + `sig` (download) and additionally `acct`,`device`,`path`,
`mode` (upload). TTL is short (5 min) and URLs are **not single-use**: a short TTL
bounds a leak, and non-single-use keeps retries safe. Tampering any bound field
breaks the signature → 400. Implementation: `internal/blob/sign.go`.

Config:

- `ABACAD_BLOB_SIGNING_KEY` — HMAC key. If unset, a random key is generated at
  boot and logged; set it explicitly to persist URLs across restarts or share a
  key across instances.
- `ABACAD_PUBLIC_BASE_URL` — scheme+host the minted URLs point at. Defaults to
  `https://<base-domain>`; override for local testing (e.g.
  `http://127.0.0.1:8848`).

## Security notes

- A signed download URL grants read of exactly one blob until it expires; a signed
  upload URL grants write of exactly one path on one device until it expires.
  Treat the URLs as secrets — do not log the `sig` query parameter.
- `GET /blobs/{id}` keeps its original bearer path (session / API key / device
  token) for callers that hold a credential; the signed path is purely additive.
  The device fetches the blob it must write using its own device token, so a
  send delivery is also account-scoped end to end.

## What did NOT change

- The **device wire protocol** is untouched: the server still sends
  `push_file{blob_id,dest_path,mode}` / `pull_file{src_path}`, and devices still
  use `/blobs` with their device token. All device clients are unaffected.
- `internal/protocol` method names and result structs are unchanged.

## Deferred

- **Large files.** A synchronous `POST /blobs/send` that blocks until the device
  confirms can outlive an ingress timeout for very large transfers. When needed,
  add an async path (`202` + a status URL) above a size threshold; the small/medium
  case keeps the one-POST-tells-you ergonomic.
- **MCP `resource_link` output** for `get_file` (needs advertising the `resources`
  capability; today only `tools` is). The URL is returned as text for now.
- **Inline convenience.** No inline `content` on `send_file` and no inline preview
  on `get_file` — a generic MCP host with no out-of-band upload can't `send_file`;
  abacad's own harness can. Revisit if third-party-host support is wanted.
