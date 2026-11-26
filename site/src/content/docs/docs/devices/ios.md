---
title: iOS and iPadOS
description: An iPhone or iPad has no VNC server. iOS VNC Bridge is a macOS menu-bar app that exposes a paired device as a standard RFB endpoint for abacad. No jailbreak.
---

An iPhone or iPad has **no VNC server** and cannot run one without a jailbreak. The hub
still lists one, because a Mac the device is paired to can bridge it:
[iOS VNC Bridge](https://github.com/abacad-ai/ios-vnc-bridge) is a menu-bar app that turns
the device into a standard RFB endpoint using Apple's developer services. The hub treats
that endpoint like any other VNC server.

```text
iPhone / iPad ──USB──▶ Mac running iOS VNC Bridge ──RFB :5970──▶ abacad ──▶ browser
```

## What the Mac needs

- macOS 26+ with **Xcode 27**, opened once (the CoreDevice developer services).
- The device **paired and trusted**, **Developer Mode** enabled, then rebooted once.
- The device **unlocked** when the session starts.
- Python 3.9+ — Xcode's toolchain is enough; the app installs `pymobiledevice3` itself.

## Start it

Install the app from
[its releases](https://github.com/abacad-ai/ios-vnc-bridge/releases): open the DMG, drag
**iOS VNC Bridge** into Applications, and launch it (right-click → Open the first time; the
build is ad-hoc signed). It lives in the menu bar.

1. **Install device runtime** — one network download, into
   `~/Library/Application Support/ios-vnc-bridge`.
2. Pick the device, keep **USB**, press **Start**. The first frame takes **15–30 seconds**
   (tunnel, stream, keyframe). With **Start when a device is found** on, the app serves as
   soon as a device is visible.
3. Copy the address and password, then add the device to the hub.

The password is generated on first launch, kept in a `0600` file in Application Support,
and always required. The app passes it to its engine over a pipe, so it never appears in
`ps`.

## Add it to the hub

Click **Add device** and fill in:

- **Name:** `iphone` or `ipad`
- **Host:** the Mac's LAN address, shown in the app
- **Port:** `5970`
- **Sign-in:** **VNC password**
- **Password:** the password from the app
- **Platform:** iOS or iPadOS
- **Mode:** `gateway`

Save, then select the device. The first authenticated session marks it **online**.

## Wi-Fi instead of USB

USB is the default: it is the connection that cannot be flaky, and pairing always needs the
cable. For the network path, enable it once with the cable in:

```bash
pymobiledevice3 lockdown wifi-connections on
```

Then switch the app's connection to **Wi-Fi** and start. Measured on an iPhone 15 Pro on the
same Wi-Fi: 34 fps under continuous motion, and a swipe takes the same ~105 ms to reach the
first framebuffer update as USB. The first touch after a long quiet spell is slower on both
connections, because the device has to send a fresh keyframe.

## What works today

| | |
| --- | --- |
| Frame rate | 25–40 fps while the screen changes |
| Frame size | About 700 px wide, a few tens of KB per frame |
| Input | Tap, drag, swipe. **Not keyboard input yet.** |
| Keep-awake | A `PreventUserIdleSystemSleep` assertion on the device, renewed every 2 minutes |
| Recovery | The session is rebuilt after a dropped tunnel; viewers are disconnected instead of shown a frozen picture |

## Troubleshooting

- **No device in the list.** Unlock the device, tap **Trust**, and press the refresh button.
  A Wi-Fi-only device appears only after "connect over network" has been enabled once.
- **Start fails immediately.** The log in the app names the failing stage — most often
  Developer Mode is off, or Xcode was never opened on this Mac.
- **The first frame never arrives.** Wait 30 seconds; the tunnel and keyframe are slow on
  first connect. A quiet screen afterwards sends almost no data, so an idle bridge looks
  idle — the log heartbeat every two seconds is the sign of life.
- **"Device session ended".** The cable was pulled, the device locked, or the tunnel
  dropped. The app rebuilds the session; viewers reconnect when a fresh frame arrives.

## Limits

- **No typing.** Tapping and dragging work; typing does not. Plan around it, or use the
  device's own on-screen keyboard where the task allows it.
- **No pinch or multi-touch.**
- **Rotation is not handled yet** — start the device in the orientation you want to view.
  iPad landscape is the first thing being fixed.
- **Protected video renders black**, as it does in any screen recording.

## Security

The bridge port is a way to drive the phone. The app always sets a password, keeps it in a
user-only file, and binds to the LAN. Scope TCP 5970 to the hub's IP in the Mac's firewall,
and treat the Mac as a privileged host: Developer Mode is what lets it inject input. The hub
stores the password in `config.yaml` like any other device credential. Read
[Security](/docs/operations/security/).
