---
title: Running a Mac unattended
description: Choose how a remote Mac should recover after restart. Keep FileVault and require a person to sign in, or configure a dedicated server Mac to log in and reconnect automatically.
---

The abacad user agent cannot expose the Mac's screen or accept injected input until
macOS has a logged-in graphical user session. abacad therefore supports two honest
restart postures.

## Secure mode

Keep **FileVault enabled**. After a restart, someone must unlock the disk and sign into
macOS before abacad can reconnect.

Use this for a personal Mac, a laptop, or any machine where protection against physical
access matters more than unattended recovery.

The abacad menu shows **Login required** under **Availability after restart** when it
detects this posture. This is not a fault: the machine is choosing security over
zero-touch recovery.

## Unattended server mode

For a dedicated Mac in a physically controlled location, you can choose automatic
recovery:

1. Turn **FileVault off**.
2. Enable **automatic login** for the account that runs abacad.
3. Enable **Start at login** in the abacad menu.
4. Set **Require password after screen saver or display off** to **Never**.
5. Prevent the Mac from sleeping automatically while the display is off.
6. Enable startup after a power failure.
7. Enable wake for network access.

The resulting sequence is:

```text
restart
→ macOS logs into the dedicated account
→ abacad starts
→ the relay reconnects
→ the Mac is available to the agent
```

:::caution
Unattended server mode removes boot-time protection. Anyone with physical access can
restart the Mac and enter that account. Use a dedicated standard account, keep personal
data and unrelated credentials off the machine, and use this mode only where the Mac is
physically controlled.
:::

## The in-app checklist

Open the abacad menu and expand **Availability after restart**. The app performs read-only
checks for:

- FileVault
- automatic login and its selected account
- the abacad login item
- password after idle
- system sleep
- startup after power loss
- wake for network access

Each row reports **configured**, **action required**, or **unknown**, followed by the
matching System Settings path. Press **Recheck** after changing a setting.

`Unknown` means the installed macOS version returned a result abacad could not interpret.
The app never assumes an unknown security setting is safe.

## Verify the whole path

Configuration is not complete until the real recovery sequence has been tested:

1. Restart the Mac.
2. Confirm it reaches the desktop without local input.
3. Confirm the device returns online in the abacad dashboard.
4. Let the display turn off and confirm the Mac remains controllable.
5. For a desktop Mac, test one real power disconnect and restoration.

macOS updates, managed-device policies, and changed account settings can alter this
posture later. Reopen the checklist whenever the Mac stops returning automatically.
