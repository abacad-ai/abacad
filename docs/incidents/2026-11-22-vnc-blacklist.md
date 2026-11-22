# 2026-11-22: TigerVNC blacklisted the hub after health probes

## Symptom

Real clients from the hub host (`172.16.1.237`) were rejected with:

```
RFB 003.003
Too many security failures
```

The TigerVNC log on `172.16.1.186:5902` showed `Connections: Blacklisted:
172.16.1.237` every 30 seconds, next to `Accepted → Closing: Clean
disconnection → 0 rects, 0 pixels`.

## Root cause

`internal/inventory.Run` probed every device every 30 seconds with a bare TCP
connection: dial, read the RFB banner, close. It never sent a client version or
authenticated. TigerVNC counts every connection that closes before
authenticating as a security failure and does not decay the counter. Once the
counter reached the default `BlacklistThreshold` (5), every later probe was
blacklisted immediately, so the lockout renewed itself forever. A fixed 30s
interval guaranteed that.

`CheckAuth` opened a second unauthenticated connection (version + security
types, no authentication) at startup and after add/edit. It was not the 30s
loop, but it had the same side effect.

## Fix

- The periodic bare-TCP probe is gone. The default health mode is `session`:
  no background connection at all. A device becomes online only after a real
  browser/agent session authenticates, and that state goes stale after 10
  minutes.
- `health: command` runs a hub-side command (the recommended SSH/systemd
  check) every 60s with exponential backoff.
- `health: vnc` is opt-in. It performs a complete RFB 3.8 handshake,
  authenticates with VncAuth or Apple DH, reads ServerInit, and disconnects.
  It is limited to one probe every five minutes and pauses for an hour after
  two authentication failures, well below TigerVNC's threshold of five.
- `Proxy` no longer opens the device connection until the client has sent its
  RFB version and (for raw agents) passed the hub token check. A browser tab
  that disappears before negotiating cannot leave an unauthenticated
  connection behind.
- Upstream security failures are forwarded to noVNC as an RFB security reason,
  so `Too many security failures` is shown to the user instead of a generic
  "connection failed".
- noVNC reconnect uses exponential backoff (1.5s → 3s → … → 5min) and stops
  automatic retries after a security failure. A fixed retry loop is what turns
  a transient outage into a rate-limit lockout.
- The device card distinguishes **online**, **offline**, and **not checked**,
  and the add/edit dialog exposes the health mode.

## Verification

- Go tests cover ten complete authenticated probes followed by a normal token
  client, blacklist reason forwarding to the downstream client, the absence of
  an upstream connection before the client version, rate limiting, and
  command-health recovery.
- Against the real Debian TigerVNC server over IPv6 (a different source IP
  from the blacklisted IPv4 address): ten `Probe` calls plus one real proxied
  client produced 12 `Accepted` lines, zero `Blacklisted` lines, and 12 clean
  disconnects with `security type VncAuth(2)`.
- After the server restart cleared the old counter, the same acceptance run
  from the real IPv4 client `172.16.1.237` passed: ten authenticated probes
  plus one normal session were all accepted, with the last `Blacklisted` line
  unchanged.

## Server-side cleanup

Raising `BlacklistThreshold` does not clear an existing in-memory failure
counter. The running `Xtigervnc` process still has to be restarted once:

```bash
sudo systemctl restart tigervncserver@:2.service
```

After that, the IPv4 client can authenticate again. With the old client
stopped and the new `health: command` check in place, no new blacklist entries
are produced.
