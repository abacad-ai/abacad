---
title: Getting started
description: Run abacad, sign in with the admin token, add your first VNC device, open it in the browser, and point an agent at the same hub.
---

This is the shortest path from nothing to a machine you can open in a browser and hand to
an agent. It assumes the machine already runs a VNC server; if it does not, the
[platform guides](/docs/devices/macos/) cover that first.

## What you need

- A machine that stays on — a Mac mini, a NAS, a Raspberry Pi, a small Linux box. This is
  the **hub**.
- Docker, or Go 1.27+ to build and run the binary. Node is only needed to build the
  frontend from source; the image already contains it, and a binary built from source
  embeds it.
- One device running a VNC server, reachable from the hub on its LAN or VPN address.

## 1. Run the hub

```bash
docker run -d --name abacad --network host \
  -v abacad-config:/config \
  ghcr.io/abacad-ai/abacad:latest

docker logs abacad
```

On Linux, `--network host` is strongly recommended: it keeps real client IPs in the hub's
logs. On macOS and Windows Docker Desktop, use
[bridge networking](/docs/install/#bridge-networking-instead-of-host) instead.

The logs print the URL and a generated admin token:

```text
  abacad is running
  → http://192.168.1.10:8787
  admin token: 3f2c...
```

The token is 64 hex characters. It authenticates the web login, the API, MCP, and every
device WebSocket. Treat it like a password.

Prefer the binary? See [Install](/docs/install/#the-binary).

## 2. Open the console

Open the printed URL in a browser and paste the admin token. The console opens on the
device rail. If you are on another machine, make sure the hub's port is reachable — a
VPN or an SSH tunnel is enough; you do not need to expose it to the internet.

## 3. Add a device

Click **Add device** and fill in:

- **Name** — what you will recognise in the rail, e.g. `Mac mini`.
- **Host** — any address the **hub** can reach: a LAN IP, a VPN address, a DNS name, or a
  forwarded local port. It is not relative to your browser.
- **Port** — the VNC server's port, usually `5900`.
- **Sign-in** — **VNC password**, or **macOS account** on a Mac. On macOS use the account
  path; the password path has no user identity and can land on the login window.
- **Platform** — used for the icon and the status hints.
- **Mode** — leave it on `gateway` unless you have a reason not to.

Save. The rail now lists the machine.

## 4. Open it

Select the machine in the rail. The screen opens in the pane next to it. The first
successful session marks the device **online**; before that it is **not seen**, because
abacad does not probe.

The viewer is noVNC's own application. Its control bar has the keyboard, clipboard,
fullscreen, scaling, and reconnect controls. Clipboard access in most browsers requires
HTTPS; see [TLS and the reverse proxy](/docs/install/#tls-and-the-reverse-proxy).

## 5. Give an agent the same hub

An agent reaches the device through MCP at `POST /mcp` with the same admin token:

```bash
claude mcp add --transport http \
  --header "Authorization: Bearer <admin token>" \
  abacad http://192.168.1.10:8787/mcp
```

The single tool, `list_devices`, returns each machine's `ws_url`; the agent opens it with
a WebSocket VNC client. If your agent does not speak MCP, the hub can generate a
ready-made `SKILL.md` instead:

1. Sign in and open **Skill** in the console.
2. Copy or download the file.
3. Put it where your agent looks for skills, e.g.
   `~/.claude/skills/abacad/SKILL.md`.

The file contains the admin token. Keep it out of git.

## What to do next

- [Install](/docs/install/) — Docker, the binary, TLS, upgrades, and the config file.
- [macOS](/docs/devices/macos/), [Windows](/docs/devices/windows/), [Linux](/docs/devices/linux/),
  [Android](/docs/devices/android/), [iOS](/docs/devices/ios/) — the server side of each platform.
- [MCP](/docs/agents/mcp/) and [the agent skill](/docs/agents/skill/) — the two ways an agent connects.
- [Security](/docs/operations/security/) — before you expose the hub beyond your LAN.
