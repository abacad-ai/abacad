---
title: Android
description: Use droidVNC-NG as the VNC server for an Android phone or tablet, handle the MediaProjection consent, and add it to abacad.
---

Use **droidVNC-NG** (F-Droid or Google Play). It is a standard VNC server with an optional
built-in noVNC page; abacad does not need that page, only the RFB port.

## Set it up

1. Install droidVNC-NG.
2. Set a password and a port (default **5900**).
3. Enable the **AccessibilityService** if you want remote input. Without it, the screen is
   view-only.
4. Tap **Start**.
5. Keep the device reachable from the hub.

Click **Add device** in the console and fill in:

- **Name:** `pixel`
- **Host:** the phone's IP address, e.g. `192.168.1.50`
- **Port:** `5900` (or the custom port you set)
- **Sign-in:** **VNC password**
- **Password:** the droidVNC-NG password
- **Platform:** Android
- **Mode:** `gateway`

## Android 10+ screen capture

Android 10 and newer require the MediaProjection consent dialog on each start. Options:

- Accept the dialog after each reboot.
- Grant it once with adb (device owner or debugging required):

  ```bash
  adb shell cmd appops set net.christianbeier.droidvnc_ng PROJECT_MEDIA allow
  ```

- Use the app's intent interface to start with a fallback capture mode, if your version
  supports it.

The consent is an Android platform rule, not a droidVNC-NG limitation. A reboot can still
need one tap.

## Power and reliability

- Disable battery optimization for droidVNC-NG.
- Enable **Start on Boot** if you want it after a reboot.
- Keep the device on a charger; Android may kill background services on battery.
- On Android 16+, some app components can opt out of accessibility input; those screens
  will not accept remote taps.

## Add it to the hub

After saving, select the phone once. The first authenticated session marks it **online**.
If it stays **not seen**, check that the hub can reach the phone's IP and that the
droidVNC-NG server is running (Android may stop it when the screen locks or the battery
saver kicks in).

## Note on "zero touch"

droidVNC-NG is a standard component, but Android's MediaProjection rules mean a reboot may
need one tap. If zero-touch Android is a hard requirement, that is the one case where a
tiny in-app VNC server is worth considering as an explicit exception. It is not part of
the hub's core.

Read [Security](/docs/operations/security/) before exposing the hub beyond your LAN.
