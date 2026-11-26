# iOS and iPadOS

An iPhone or iPad has **no VNC server** and cannot run one without a jailbreak.
The hub still lists one, because a Mac the device is paired to can bridge it:
[iOS VNC Bridge](https://github.com/abacad-ai/ios-vnc-bridge) turns the device
into a standard RFB endpoint using Apple's developer services, and the hub
treats that endpoint like any other VNC server.

```
iPhone / iPad ──USB──▶ Mac running iOS VNC Bridge ──RFB :5970──▶ abacad ──▶ browser
```

## Start it

Install the menu-bar app from
[its releases](https://github.com/abacad-ai/ios-vnc-bridge/releases), open it, and
follow its first-run flow: install the device runtime once, pick the device,
press **Start**. The app shows the address and the password to use.

Full setup, requirements and troubleshooting live in the app's own docs:
<https://github.com/abacad-ai/ios-vnc-bridge/tree/main/docs/ios.md>.

## Add it to the hub

- **Name:** `iphone` or `ipad`
- **Host:** the Mac's LAN address, shown in the app
- **Port:** `5970`
- **Sign-in → VNC password:** the password from the app
- **Platform:** iOS (or iPadOS)
- **Mode:** Gateway

Agents reach it through the hub's `/ws/vnc/<id>` like any other device.

## Limits

- Tap, drag and swipe; no keyboard, no pinch.
- Rotation is not handled yet — start the device in the orientation you want.
- The device must stay reachable; the app rebuilds the session when the link
  returns and disconnects viewers in the meantime.
- Wi-Fi works after "connect over network" has been enabled once with the cable
  in.
