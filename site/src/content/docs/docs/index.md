---
title: What is abacad
description: abacad is a small, self-hosted VNC hub. It connects the VNC servers your machines already run to one browser console and to one WebSocket per device for agents.
---

abacad gives the VNC servers already running on your machines **one home**. You open
any of them in a browser. An agent opens the same stream with the same admin token.
There is no software to install on the devices, no cloud, and no relay.

```text
  macOS Screen Sharing ─┐
  Linux x11vnc/wayvnc  ─┤
  Windows TigerVNC     ─┼── RFB ──▶  abacad hub  ── HTTPS/WSS ──▶ browser (noVNC)
  Android droidVNC-NG  ─┤              │
  iOS/iPadOS via the Mac app─┘         └── same WSS ──▶ agent's VNC client
```

<dl class="glance">
  <div><dt>On the device</dt><dd>Nothing from abacad</dd></div>
  <div><dt>Between hub and device</dt><dd>Standard RFB</dd></div>
  <div><dt>To the client</dt><dd>One WebSocket per device</dd></div>
  <div><dt>To manage</dt><dd>One port, one token</dd></div>
</dl>

## One stream, two clients

The browser and the agent are not two integrations. They are two clients of the same
WebSocket, at `/ws/vnc/<id>`, carrying plain RFB — the websockify convention that noVNC
already speaks. The browser gets the vendored noVNC application. The agent gets the bytes
and uses any VNC client it likes.

That is the whole product. abacad is not a new device API and not a second screen
protocol. It is the smallest thing that lets a human and an agent reach the VNC servers
you already trust.

Every viewer is its own RFB session. A person and an agent can be connected at the same
time only if the VNC server accepts more than one viewer. `x11vnc` and TigerVNC do with
`-shared`; some servers do not. If yours does not, close the agent's session, finish the
step in the browser, and reconnect the agent. The device state is the same either way.

## What it is not

- **Not a hosted service.** You run the hub and own the network path. There is no
  account, no relay, and no vendor between you and the device.
- **Not a remote-desktop reimplementation.** The screen stays the VNC server's; abacad
  does not decode the framebuffer, record it, or synthesize input.
- **Not a discovery service.** There is no mDNS, no scan, no advertised list. A machine
  exists to abacad only after you add its host and port.
- **Not an approval layer.** In `gateway` mode the hub signs in to the device for the
  client; it does not ask you to approve each action. If you need that, put the agent
  behind your own policy.

## Modes

Each device has a mode. The default is `gateway`.

| Mode | Browser and agents | Credential handling |
| --- | --- | --- |
| `gateway` (default) | Yes, through the hub's WebSocket | The hub stores the device password. Clients see no VNC password. |
| `l4` | Yes; the client answers the device's VNC password | The hub is a blind pipe. No password storage is needed. |
| `direct` | No; catalog only | The agent connects to the device directly; the hub carries no VNC traffic. |

In `gateway` mode the hub terminates only the RFB handshake. After that it is a blind
byte pipe. On macOS it performs Apple's account sign-in (RFB type 30), which attaches the
session to the logged-in user's console instead of the `loginwindow` session.

<a id="online"></a>

## Online status

abacad **never opens a connection to a device on its own**. There is no scheduled check,
no health command, and no **Check** button. A device is marked **online** only after a
real browser or agent session authenticates to it, and that state goes stale after ten
minutes. A device you have never opened shows **not seen**.

This is deliberate. TigerVNC counts every connection that closes before authenticating as
a security failure, and its counter does not decay: even a well-behaved probe can
eventually blacklist the hub. The only connection abacad makes is the one you or an agent
asked for. See [Troubleshooting](/docs/operations/troubleshooting/) if you inherit a
blacklist.

## Platforms

Each platform guide covers the VNC server, the credentials, the firewall, and the parts
that usually go wrong. These are five real screens from one hub, in the same browser
console:

<div class="platform-gallery">
  <a class="platform-gallery-item" href="/docs/devices/macos/">
    <img src="/screens/live-macos-thumb.webp" alt="A Mac mini desktop captured over VNC" loading="lazy" />
    <span>macOS</span>
  </a>
  <a class="platform-gallery-item" href="/docs/devices/windows/">
    <img src="/screens/live-windows-thumb.webp" alt="A Windows 11 desktop captured over VNC" loading="lazy" />
    <span>Windows</span>
  </a>
  <a class="platform-gallery-item" href="/docs/devices/linux/">
    <img src="/screens/live-linux-thumb.webp" alt="A Debian lock screen captured over VNC" loading="lazy" />
    <span>Linux</span>
  </a>
  <a class="platform-gallery-item" href="/docs/devices/android/">
    <img src="/screens/live-android-thumb.webp" alt="An Android home screen captured over VNC" loading="lazy" />
    <span>Android</span>
  </a>
  <a class="platform-gallery-item" href="/docs/devices/ios/">
    <img src="/screens/live-iphone-thumb.webp" alt="An iPhone home screen captured over VNC" loading="lazy" />
    <span>iOS</span>
  </a>
</div>

<div class="platform-cards">
  <a class="platform-card" href="/docs/devices/macos/">
    <strong>macOS</strong>
    <span>Screen Sharing, with a macOS account for the console session</span>
  </a>
  <a class="platform-card" href="/docs/devices/windows/">
    <strong>Windows</strong>
    <span>TigerVNC in service mode, including the login screen</span>
  </a>
  <a class="platform-card" href="/docs/devices/linux/">
    <strong>Linux</strong>
    <span>x11vnc, TigerVNC, wayvnc, and the Wayland alternatives</span>
  </a>
  <a class="platform-card" href="/docs/devices/android/">
    <strong>Android</strong>
    <span>droidVNC-NG, with the MediaProjection caveat</span>
  </a>
  <a class="platform-card" href="/docs/devices/ios/">
    <strong>iOS &amp; iPadOS</strong>
    <span>A paired Mac runs iOS VNC Bridge; no jailbreak</span>
  </a>
</div>

## Next

- [Getting started](/docs/getting-started/) — run the hub and connect the first machine.
- [Install](/docs/install/) — Docker, the binary, TLS, and the reverse proxy.
- [MCP](/docs/agents/mcp/) — point an agent at `POST /mcp`.
- [Security](/docs/operations/security/) — the threat model and the network boundary.
