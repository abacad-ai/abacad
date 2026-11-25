---
title: Agent skill
description: If your agent does not speak MCP, the /skill page generates a ready-made SKILL.md with the hub address, admin token, and machine list baked in.
---

The **Skill** page is the no-integration path. It generates a plain `SKILL.md` for an
agent: the hub address, the admin token, the machine list, and the rules that keep the hub
healthy. There is no server-side component and nothing to install on the hub.

## Generate it

1. Open the console and sign in.
2. Open **Skill** in the navigation rail.
3. Check the hub address. It defaults to the address in your browser, which is usually
   what you want; a reverse proxy or a second LAN name can be typed in.
4. Copy or download `SKILL.md`.

The file is generated in the browser from the hub's API. The admin token is read from the
signed-in session and baked into the file.

## Put it where the agent looks

One folder per skill:

```text
~/.claude/skills/abacad/SKILL.md
~/.dsh/skills/abacad/SKILL.md
```

Any agent that loads a skills directory can use it. The file names the hub, carries the
token, and lists each machine with its `ws_url` and what the VNC handshake expects.

## What it contains

- The hub origin and whether it is `http` or `https`.
- The admin token, so the agent needs nothing else.
- A table of machines: name, id, platform, endpoint, password requirement, state.
- The connection convention: each machine is a WebSocket at `/ws/vnc/<id>` carrying plain
  RFB. The agent can open it directly or bridge it to a local TCP port with `websocat`.
- The health rules: do not probe devices, do not open sessions just to check status, and
  remember that the hub is a single ingress.

## Treat it as a password

The file contains the admin token, which opens every machine and the hub's settings. Keep
it out of git and out of shared drives. If it leaks, rotate `admin_token` in the config and
restart the hub; every existing session and old copy of the skill stops working.

## Skill or MCP?

- **MCP** is better when the agent can speak Streamable HTTP and you want the device list
  to stay current without regenerating a file.
- **Skill** is better when the agent only knows files, when you want a self-contained
  hand-off, or when you are using an agent that cannot add an MCP server.

Both reach the same WebSocket. Pick one; there is no need to wire both.
