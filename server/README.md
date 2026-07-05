# Abacad server (v0)

One Node process that is **both** the MCP endpoint an agent (e.g. Claude Code) talks to
**and** the WebSocket relay the Android device dials into. LAN-only, single device, no auth
— the minimum loop that lets a remote agent drive the phone. Later this splits into
`relay/` + `mcp/` + `contract/`, and the relay moves to a public host for internet reach.

```
Claude Code ──MCP (HTTP :8848/mcp)──▶  this server  ──WS (:8848/device)──▶  Abacad Android app
```

Tools exposed to the agent: `screenshot(include_ui_tree)`, `tap`, `long_press`, `swipe`,
`input_text`, `back`, `home`, `recents`. `screenshot` returns the PNG and, by default, the
accessibility UI tree in the same call. Waking a dark/locked screen is automatic (handled on
the device) — there is no wake/sleep tool; the phone sleeps on its own display timeout.

## Run

```bash
npm install
npm start          # listens on :8848 ; prints the endpoints
```

Then point the Android app's server URL at `ws://<this-machine-LAN-IP>:8848/device`
(the app and this machine must be on the same Wi-Fi), enable the accessibility service,
and register the MCP endpoint with your agent:

```bash
claude mcp add --transport http abacad http://localhost:8848/mcp
```

Now ask the agent to `screenshot` (image + UI tree) / `tap` / `input_text` the phone.

Check status any time: `curl http://localhost:8848/health` → `{ok, deviceConnected}`.

## Verify without a phone

```bash
npm install
npm run typecheck          # compiles
node mock-device.mjs &     # fake device on /device
npm start &                # the server
node smoke.mjs             # acts as the agent; prints "SMOKE OK"
```

## Notes / not yet

- **Stateless MCP** (`sessionIdGenerator: undefined`). If a client needs session IDs, switch
  `src/index.ts` to stateful mode.
- **Cleartext `ws://` on LAN** — fine here; a hosted deployment should use `wss://`.
- Not in v0: cloud relay / NAT traversal, auth/pairing, approval gating, tap-by-node-id,
  `open_app`. Each is additive behind the same `src/protocol.ts` contract.
