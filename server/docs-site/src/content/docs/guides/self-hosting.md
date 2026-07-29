---
title: Run your own relay
description: Point abacad clients at a relay you operate. What self-hosting actually buys you (and what it doesn't), how to run the server, and the one security detail that changes when a device enrolls itself.
---

Every abacad client talks to a **relay**: the server devices dial out to, and the one an
agent connects through. By default that's `abacad.ai`. You can run your own instead — the
relay is a single Go binary with a SQLite database.

## What this actually buys you

Be clear-eyed about it, because "self-host for better performance" is only sometimes true.

**Genuine wins:**

- **Lower latency on your own network.** If the agent and the devices are both on your
  LAN, traffic stays on your LAN instead of crossing the internet twice.
- **Control of the control plane.** Command frames, the audit trail, screenshots and
  transferred files all stay on infrastructure you own. See
  [Security & trust](/security/) for what the relay can and cannot see.
- **No dependency on our uptime**, and no account on someone else's service.

**What it does *not* automatically buy you:**

- **Better reachability.** A relay behind a home NAT or a dynamic IP can easily be *worse*
  reachable than the hosted one. The relay must have a stable, publicly reachable address
  for devices to dial in from anywhere — that's the whole job it does.
- **Less work.** You become the operator: TLS certificates, backups, patching, and
  monitoring are yours. The split is spelled out in the shared-responsibility model.

If your devices and your agent are in the same building, self-hosting is usually a clear
win. If they're scattered across the internet and you don't already run public
infrastructure, the hosted relay is probably the better tool.

## Running the relay

The server needs a stable hostname, a TLS certificate, and one open port. Terminate TLS
with a reverse proxy (Caddy or nginx) and forward to the binary:

```sh
abacad-server \
  -addr 127.0.0.1:8848 \
  -base-domain relay.example.com \
  -db /var/lib/abacad/abacad.db
```

`-base-domain` (or `ABACAD_BASE_DOMAIN`) is the domain devices are addressed under — it
drives the SSH jump hostnames and browser-device subdomains, so set it to the name your
proxy actually serves.

**TLS is not optional.** Every native client refuses a plaintext `ws://` relay unless it
resolves to loopback, so a relay without a certificate simply won't accept devices.

### Your proxy must set `X-Forwarded-For`

The server reads the client's address from the left-most `X-Forwarded-For` hop and trusts
it. That address is used for two things: per-IP rate limiting on sign-in and enrollment,
and the "from where" column in your dashboard's activity trail.

So the proxy in front of the relay must **overwrite** that header with the real peer
address, not append to it. If it appends — or if you expose the binary directly with no
proxy at all — any client can send its own `X-Forwarded-For` and choose the address that
lands in your audit log and your rate limiter. The trail will look completely normal
while attributing actions to an address the caller picked.

Caddy's `reverse_proxy` does the right thing by default. On nginx, set it explicitly:

```nginx
proxy_set_header X-Forwarded-For $remote_addr;   # NOT $proxy_add_x_forwarded_for
proxy_set_header X-Forwarded-Proto $scheme;
proxy_set_header X-Forwarded-Host $host;
```

`$proxy_add_x_forwarded_for` **appends** to whatever the client sent, which leaves the
client's forged value in the left-most position — exactly the case to avoid. Use
`$remote_addr` unless you run a further trusted proxy in front, in which case that one
must be the thing sanitising the header.

The SSH jump host is unaffected: it reads the peer address straight off the socket, so
those rows are always the true origin.

### Optional: country and city in the activity trail

The relay can record the country and city an activity came from, so an unfamiliar entry
reads as "from a country I've never worked from" rather than a bare number. This is off
by default and needs a MaxMind database, which **abacad does not ship** — MaxMind's terms
don't permit redistributing it.

1. Create a free [MaxMind account](https://www.maxmind.com/en/geolite2/signup) and
   generate a licence key.
2. Download the **GeoLite2 City** database (`.mmdb`, not the CSV).
3. Point the relay at it:

```sh
abacad-server -geoip-db /var/lib/abacad/GeoLite2-City.mmdb   # or ABACAD_GEOIP_DB
```

The file is read once at boot, so **re-download it periodically and restart** — MaxMind
republishes weekly and a stale database drifts. If the path is missing or unreadable the
relay logs `geoip DISABLED: …` and starts anyway: activity rows then carry an IP and no
location, which is the same as running without geo at all. Nothing else degrades.

**Country is dependable; city is not.** City-level geolocation is regularly the wrong
city in the right region, and for mobile carriers, VPNs and CGNAT it can be off by a
country. The dashboard shows the country code first for that reason. Treat a surprising
city as a prompt to look at the IP and the credential, not as a finding on its own.

Private, loopback and CGNAT addresses are never looked up, so on a relay reached over a
LAN or VPN these columns stay empty by design. And because the location is derived from
the same `X-Forwarded-For` described above, it inherits that header's trust: if the proxy
appends instead of overwriting, the country is as forgeable as the address it came from.

## Pointing clients at it

Each client has a **relay** setting. Change it and the client re-enrolls against the new
relay, showing a fresh device ID and claim code.

```sh
abacad --relay https://relay.example.com
```

Your relay serves its own dashboard, so you claim devices at
`https://relay.example.com/claim` and manage them there. Accounts are per-relay; devices
enrolled on one relay are invisible to another.

## The one security detail that changes

This is the part worth reading carefully, because it is genuinely weaker than the hosted
path and we would rather say so than let you discover it later.

When a client self-enrolls, it dials a relay address it was simply *configured* with.
Nothing has told the client what that server's identity should be, so the first connection
is **trust-on-first-use**: the client validates the TLS certificate, but it has no
independent way to know it reached *your* relay rather than something that answered for
that name.

- **With a publicly-trusted certificate** (Let's Encrypt and friends), a normal CA chain
  authenticates the server, and this is the same posture as any HTTPS service. **This is
  the configuration we recommend, and it needs nothing extra.**
- **With a self-signed certificate, a private CA, or a bare IP** — typically a LAN-only
  relay — there is no out-of-band channel telling the client what to expect. For those
  deployments, enrol with `abacad connect` instead: it carries the server's identity
  across an already-authenticated browser session rather than assuming it.

Never configure a client to ignore certificate errors to "make it work". A relay that
can't prove who it is can read every command frame in your control plane.

## Where to go next

- [Security & trust](/security/) — what the relay sees, and what stays end-to-end opaque.
- [Transport](/reference/transport/) — how the device connection and tunnels actually work.
- [SSH access](/guides/ssh/) — reaching a device's own `sshd` through your relay.
