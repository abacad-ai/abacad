---
title: iOS and iPadOS
description: An iPhone or iPad has no VNC server. A Mac you already pair it to runs the iosvnc bridge, which exposes the device as a standard RFB endpoint for abacad. No jailbreak.
---

An iPhone or iPad has **no VNC server** and cannot run one without a jailbreak. The hub
still lists one, because a Mac the device is paired to can bridge it: [`iosvnc`](https://github.com/abacad-ai/abacad/tree/main/iosvnc)
turns the device into a standard RFB endpoint using Apple's developer services, and the
hub treats that endpoint like any other VNC server.

```text
iPhone ──USB──▶ Mac running iosvnc ──RFB :5970──▶ abacad ──▶ browser
```

## What the Mac needs

- macOS with **Xcode 27** (the CoreDevice developer services).
- The device **paired and trusted**, **Developer Mode** enabled.
- The device **unlocked** and on **USB** when the bridge starts.
- Python 3.9+ with `pymobiledevice3` in a venv next to the bridge.
- **No root**: on macOS the bridge reaches the device over the in-process userspace
  tunnel.
- Go 1.27+ to build the bridge (`serve-ios.sh` does it for you).

## Start the bridge

From the repository's `iosvnc/` directory:

```bash
cd iosvnc
python3 -m venv .venv && .venv/bin/pip install pymobiledevice3
./serve-ios.sh 192.168.1.10:5970 --password Abcd1234
```

The first frame takes **15–30 seconds** (tunnel, stream, keyframe). Once it is up the
bridge holds a power assertion on the device, so the screen does not lock mid-session.

Always pass `--password` when the bridge listens on a LAN address. Without it, anyone who
can reach port 5970 can drive the phone.

## Add it to the hub

Click **Add device** and fill in:

- **Name:** `iphone`
- **Host:** the Mac's LAN address, e.g. `192.168.1.10`
- **Port:** `5970`
- **Sign-in:** **VNC password**
- **Password:** the `--password` you passed to the bridge
- **Platform:** iOS
- **Mode:** `gateway`

Save, then select the device. The first authenticated session marks it **online**.

## Wi-Fi instead of USB

The device can be reached over the network instead of the cable:

```bash
pymobiledevice3 lockdown wifi-connections on   # once, with the cable in
./serve-ios.sh 0.0.0.0:5970 --password Abcd1234 --connection wifi
```

The switch is the standard "connect to this iPhone over network" one. It makes usbmuxd
list the device twice, once per interface, and `--connection wifi` builds the whole
session (tunnel, display stream, HID) from the network entry. Measured on an iPhone 15
Pro on the same Wi-Fi: 34 fps under continuous motion, and a swipe takes the same ~105 ms
to reach the first framebuffer update as USB. The first touch after a long quiet spell is
slower on both connections, because the device has to send a fresh keyframe.

USB stays the default: it is the connection that cannot be flaky, and pairing still
requires a cable.

## What works today

| | |
| --- | --- |
| Frame rate | 25–40 fps while the screen changes |
| Frame size | About 78 KB (Tight/JPEG, 592×1280 for an iPhone 15 Pro) |
| Input | Tap, drag, swipe. **Not keyboard input yet.** |
| Keep-awake | A `PreventUserIdleSystemSleep` assertion on the device, renewed every 2 minutes |
| Viewers | One viewer per session |

Keyboard input is the notable gap: tapping and dragging work, typing does not. Plan around
it for tasks that need text entry, or use the device's own on-screen keyboard where the
task allows it.

## Troubleshooting

- **The bridge cannot find the device.** Unlock it, trust the pairing, enable Developer
  Mode, and reconnect the cable. Check `iosvnc devices`.
- **The first frame never arrives.** Wait 30 seconds; the tunnel and keyframe are slow on
  first connect. If it still fails, restart the bridge and watch its logs.
- **The session drops after the screen goes quiet.** A quiet screen sends no frames; the
  bridge logs a heartbeat every two seconds. If the log stops, the bridge is wedged and
  restarts. That is deliberate: a dead session is never shown as live.
- **The picture is frozen.** A frozen frame and a still screen look the same. The bridge is
  the source of truth; when it ends, the hub closes the viewer instead of leaving a stale
  image.
- **The device locks.** The bridge holds an assertion, but a manual lock still wins. Keep
  the device unlocked and on power for long sessions.

## Security

The bridge port is a way to drive the phone. Always set `--password`, bind it to a private
interface, and scope TCP 5970 to the hub's IP in the Mac's firewall. The hub stores that
password in `config.yaml` like any other device credential. Read
[Security](/docs/operations/security/).
