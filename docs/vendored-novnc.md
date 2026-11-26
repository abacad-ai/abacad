# Vendored noVNC

`frontend/public/novnc` is the upstream [noVNC](https://github.com/novnc/noVNC)
**application** at **1.7.0** — the UI and the RFB client together, not the npm
package.

The npm package cannot supply it: `@novnc/novnc` publishes only `core/` and
`vendor/` (`"files": ["core", "vendor", ...]`, `"exports": "./core/rfb.js"`), so
the control bar, its stylesheet, the locale files and the images are not in it.
The application is also not a component — it is a page that owns the DOM.
Proxmox VE solves the same problem the same way: ship the application under
`/usr/share/novnc` and point an iframe at it. abacad ships it under `/novnc`.

## What is here

| Path | Why |
| --- | --- |
| `vnc.html` | UI entry point; its relative paths keep `app/` and `core/` together |
| `app/` | UI, styles, images, locales, sounds |
| `core/` | RFB client (the same code the npm package publishes) |
| `vendor/pako/` | inflate/deflate |
| `defaults.json`, `mandatory.json` | read by `vnc.html` at startup |
| `package.json` | only its version is read, to label the settings panel |
| `AUTHORS`, `LICENSE.txt` | required by the license |

Not copied: `vnc_lite.html`, `docs/`, `po/`, `tests/`, `utils/`, `snap/`,
`eslint.config.mjs`, `karma.conf.cjs`. Apart from the one change listed under
[Patches](#patches) the tree is the release tarball, so an upgrade is a copy, not
a merge. (The one earlier attempt to patch `core/rfb.js` was reverted for exactly
this reason; see commit `898b0d7`.)

## Patches

| File | Change |
| --- | --- |
| `app/ui.js` | `updateBeforeUnload()` only removes the `beforeunload` listener; it never registers it |

noVNC arms a `beforeunload` handler whenever a session connects, so closing the
tab popped the browser's "Leave site?" — for a remote screen, with nothing typed
into the page to lose. A screen is a view of a device, not a document, and the
prompt only stood between the human and the tab they meant to close. The prompt
is therefore off: `handleBeforeUnload` keeps its upstream code, and the single
dropped registration is the whole diff.

A re-vendor copies the tarball over that line and quietly brings the prompt
back, so `frontend/scripts/check-novnc-patches.mjs` (run by `npm run lint`, and
so by CI) fails when any file under `public/novnc` registers a `beforeunload`
handler again.

## How it is served

`vite build` copies `frontend/public/` into `internal/web/dist`, the Go binary
embeds that directory, and `internal/web/server.go` serves it at `/novnc/`
(`/novnc` and `/novnc/` redirect to `/novnc/vnc.html`). The route is only
registered when `dist/novnc/vnc.html` exists, so a frontend built without the
vendored tree still runs.

`frontend/src/components/vnc-frame.tsx` points an iframe at
`/novnc/vnc.html?autoconnect=1&path=ws(s)://<hub>/ws/vnc/<id>&resize=scale…` and
otherwise stays out of the way. The device rail, the login and the device API
are ours; keys, clipboard, fullscreen, scaling and reconnect are noVNC's.

## Licenses

`core/` is MPL-2.0; `app/` is 2-clause BSD. Both are unchanged and both licenses
travel with the tree (`LICENSE.txt`, and the header in every `app/` file).

## Upgrading

```bash
V=1.7.0
tmp=$(mktemp -d)
curl -sL "https://codeload.github.com/novnc/noVNC/tar.gz/refs/tags/v${V}" | tar -xz -C "$tmp"
rm -rf frontend/public/novnc && mkdir -p frontend/public/novnc
cp -R "$tmp/noVNC-${V}"{/app,/core,/vendor,/vnc.html,/defaults.json,/mandatory.json,/package.json,/AUTHORS,/LICENSE.txt} frontend/public/novnc/
# Re-apply the patch from the table above, then let the check confirm it:
node frontend/scripts/check-novnc-patches.mjs
npm --prefix frontend run build
```

Then open a device and check that the control bar still appears, a session
connects, the keyboard works, and closing the tab asks nothing. The version
label in noVNC's settings panel comes from the copied `package.json`, so it
proves the tree is consistent.

Adding a file to the app (a new theme, an extra locale) is a patch: prefer
upstreaming it to noVNC and bumping the version instead.
