# iOS and iPadOS

An iPhone or iPad has **no VNC server** and cannot run one without a jailbreak.
The hub still lists one, because a Mac the device is paired to can bridge it:
[`iosvnc`](../../iosvnc) turns the device into a standard RFB endpoint using
Apple's developer services, and the hub treats that endpoint like any other VNC
server.

```
iPhone ──USB──▶ Mac running iosvnc ──RFB :5970──▶ abacad ──▶ browser
```

## What the Mac needs

- macOS with **Xcode 27** (the CoreDevice developer services)
- the device **paired and trusted**, **Developer Mode** enabled
- the device **unlocked** and on **USB** when the bridge starts
- Python 3.9+ with `pymobiledevice3` in a venv next to the bridge
- **no root**: on macOS the bridge reaches the device over the in-process
  userspace tunnel

## Start the bridge

```bash
cd iosvnc
python3 -m venv .venv && .venv/bin/pip install pymobiledevice3
./serve-ios.sh 192.168.1.10:5970 --password Abcd1234
```

The first frame takes 15–30 s (tunnel, stream, keyframe). Once it is up the
bridge holds a power assertion on the device, so the screen does not lock
mid-session.

## Add it to the hub

Click **Add device** and fill in:

- **Name:** `iphone`
- **Host:** the Mac's LAN address, e.g. `192.168.1.10`
- **Port:** `5970`
- **Sign-in → VNC password:** the `--password` you passed to `serve-ios.sh`
- **Platform:** iOS (or iPadOS)
- **Mode:** Gateway

Agents reach it through the hub's `/ws/vnc/<id>` like any other device; the
per-device raw VNC ports are gone.

## Limits

- **No typing.** Tap, drag and swipe work; there is no keyboard path yet.
- **25–40 fps**, ~78 KB per frame at 592×1280. The device only encodes when the
  screen changes, so a still screen costs almost nothing.
- **The device must stay connected over USB.** Pulling the cable ends the
  session; the bridge does not reconnect on its own yet.
- **Protected video renders black**, as it does in any screen recording.
- It drives the device through **Developer Mode**, so treat the Mac as a
  privileged host: keep the RFB port on the LAN and always set a password.
