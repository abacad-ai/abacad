# Android

Use **droidVNC-NG** (F-Droid or Google Play). It is a standard VNC server with
mDNS advertising and an optional built-in noVNC page.

## Set it up

1. Install droidVNC-NG.
2. Set a password and a port (default **5900**).
3. Enable the **AccessibilityService** if you want remote input.
4. Tap **Start**.
5. Keep the device on the same Wi-Fi/LAN as the hub.

The app advertises `_rfb._tcp` via Zeroconf, so the hub's **Discovered** section
may offer it automatically. Otherwise add it by IP:

```bash
abacad add --name pixel --host 192.168.1.50 \
  --password 'the-droidvnc-password' --platform android --expose
```

## Android 10+ screen capture

Android 10 and newer require the MediaProjection consent dialog on each start.
Options:

- Accept the dialog after each reboot.
- Grant it once with adb (device owner/debugging required):

  ```bash
  adb shell cmd appops set net.christianbeier.droidvnc_ng PROJECT_MEDIA allow
  ```

- Use the app's intent interface to start with a fallback capture mode, if your
  version supports it.

## Power and reliability

- Disable battery optimization for droidVNC-NG.
- Enable **Start on Boot** if you want it after a reboot.
- Keep the device on a charger; Android may kill background services on battery.
- On Android 16+, some app components can opt out of accessibility input; those
  screens will not accept remote taps.

## Note on "zero touch"

droidVNC-NG is a standard component, but Android's MediaProjection rules mean a
reboot may need one tap. If zero-touch Android is a hard requirement, that is the
one case where a tiny in-app VNC server (the v1 approach) is worth considering as
an explicit exception. It is not part of the hub's core.
