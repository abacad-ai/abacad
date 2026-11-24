# macOS

macOS ships a VNC-compatible server with Screen Sharing. abacad can sign in two
ways, and the difference matters:

| Sign-in | RFB type | What you get |
|---|---|---|
| **macOS account** (recommended) | 30 (Apple DH) | The logged-in user's console session. Keyboard and mouse reach the desktop and the lock screen. |
| VNC password | 2 (classic DES) | A session with no user identity. On current macOS it commonly lands on the `loginwindow`, which is why the viewer can show a login screen frozen at the time the user logged in and ignore input. |

The rest of this page uses **macOS account** sign-in.

## Enable it

1. **System Settings → General → Sharing → Screen Sharing**, turn it on.
2. Open the **ⓘ** next to Screen Sharing → **Computer Settings**.
   **VNC viewers may control screen with password** is optional for abacad. It
   only adds the legacy type 2 path; you can leave it off.
3. Make sure the account is allowed to control the screen. With plain Screen
   Sharing this is usually all users. With **Remote Management**, add the
   account explicitly and give it Observe/Control.
4. Make sure the Mac's firewall allows Screen Sharing (System Settings →
   Network → Firewall → Options).

The server listens on TCP **5900**.

## Add it to the hub

In the hub web UI, click **Add device**:

- **Name:** `Mac mini`
- **Host:** the Mac's IP address, e.g. `192.168.1.92`
- **Port:** `5900`
- **Sign-in:** **macOS account**
- **macOS username:** the short name (`whoami`), not the display name
- **Password:** the password you use to log in to the Mac, not the VNC password
- **Platform:** macOS
- **Also open a raw VNC port for agents:** check it if an agent should connect
  with a standard VNC client; leave it off for browser-only.

The card should turn **online**. If it shows
`macOS: switch Sign-in to "macOS account"`, the device is still using the
legacy VNC password path; edit it and switch.

## Why the VNC password is not enough

Apple's Screen Sharing server accepts three families of authentication:

- **type 30 (Apple DH)** — a real macOS account name and password, exchanged
  with Diffie-Hellman and encrypted with AES-128-ECB. The server knows **which
  user** is connecting, so it can attach the session to that user's console.
- **type 33/35/36** — Apple's RSA tunnel and related variants. noVNC-ARD uses
  these; abacad uses type 30 because it is the simplest interoperable one.
- **type 2 (classic VNC)** — one machine-wide password with no user name.

The type 2 path has no identity to attach to. On current macOS it can land on
the **loginwindow** session, which is a real, separate session that keeps
rendering the last login screen. That produces exactly the confusing picture:

- the viewer shows a login screen, often with a clock frozen at the time the
  user logged in;
- the Mac answers ping and accepts TCP 5900;
- clicking and typing appear to do nothing, because they are going to the
  inactive `loginwindow` session, not the user's desktop.

This is **not** a display-sleep problem, and it is not the hub dropping
keystrokes. It is the authentication path choosing the wrong session. This is
also why [noVNC-ARD](https://github.com/peetinc/noVNC-ARD) works on the same
Mac: it authenticates with type 30/33 and then explicitly selects the console
session.

abacad's hub performs the type 30 exchange itself and still presents a plain
`None` handshake to the browser. noVNC does not need to know about Apple DH, and
the account password never reaches the browser. The raw VNC endpoint works the
same way: the agent uses the hub token, and the hub signs in to the Mac.

## If the Mac is locked

The hub has already signed in as the account, so the connection is attached to
the right session. The macOS lock screen still asks for the account password
before it unlocks:

1. Click once inside the viewer (or press **Space**) to reveal the password
   field.
2. Type the macOS account password and press Return.
3. If the keyboard does nothing, click **Keyboard** in the viewer's top bar and
   use noVNC's on-screen keyboard.

The lock screen is a secure-input surface; some macOS versions accept synthetic
events only after the ARD session is established, which is what type 30 does.

## If the screen really is asleep

A frozen picture can also mean the Mac's display is asleep. The Mac can answer
ping and TCP 5900 while the framebuffer stays on its last frame. Check this
only after confirming the device uses **macOS account** sign-in.

Wake it physically or with Apple's Screen Sharing app, then reconnect. To keep a
headless Mac awake:

```bash
# Never sleep the system or display; wake on magic packet.
sudo pmset -a sleep 0 displaysleep 0 womp 1
```

Or run `caffeinate -d -s` while the Mac should remain available. On a desktop,
System Settings → **Energy** also has **Prevent automatic sleeping when the
display is off**.

For a lock screen you never want to fight, System Settings → **Lock Screen** →
**Require password after screen saver begins or display is turned off** → Never.
That is a security trade-off; only do it on a machine you already trust and
control access to.

## Screen Sharing vs Remote Management

If **Remote Management** is enabled, it owns the login window and its own access
list. Type 30 can still work, but the account must be listed there with
Observe/Control. If authentication fails with
`Authentication or authorization failure`, the DH exchange already succeeded
and the Mac refused the account — check Remote Management's **Allow access for**
list, not the password.

For a simple setup, prefer:

- **Screen Sharing** on, **Remote Management** off;
- the macOS account you will use is an admin or is allowed to control the
  screen;
- the legacy VNC password checkbox is optional.

Switching from Remote Management to Screen Sharing is a configuration change,
not a workaround: it removes Apple's managed login session from the path.

## Keep it reachable

- Prevent sleep: **System Settings → Energy Saver → Prevent automatic sleeping
  when the display is off**, or run `caffeinate -s` while the hub needs it.
- If the Mac is on Wi-Fi, give it a DHCP reservation so the host field stays
  valid.

## Known issues

- The type 30 exchange uses a 4096-bit prime. The first connect can take a
  moment longer than a VNC-password connect; that is normal.
- The hub stores the macOS account password in `config.yaml` (mode `0600`). If
  that is not acceptable, create a dedicated local macOS account for remote
  control and use that. It is the same trust model as storing the VNC password,
  with a more capable credential.
- Screen Sharing may show the login window after a reboot. That is the
  `loginwindow` session, and type 30 is still the way to sign in to the right
  account.
- Only one or a few viewers may be accepted depending on the macOS version and
  the shared flag. If an agent and a browser conflict, use one at a time.
- Apple's own Screen Sharing app prefers Apple's protocol when talking to
  another Mac. From abacad, use the browser/noVNC path or a standard VNC client;
  both are ordinary RFB after authentication.
