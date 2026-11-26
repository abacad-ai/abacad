# abacad.ai

The public site: a static Astro 7 app with a custom landing page at `/` and
Starlight documentation at `/docs/`. It is deployed as plain files; there is no
server-side code and no dependency on the hub.

## Commands

```bash
cd site
npm ci
npm run dev       # http://localhost:4321
npm run build     # writes dist/ and checks every internal link and anchor
npm run preview   # serve the built files
npm run check     # astro check (types, content schema)
npm run og        # regenerate public/og.png and public/og-zh.png
```

Node 24 is required. The site is independent of the Go binary.

## Structure

```text
src/pages/index.astro            English landing
src/pages/zh/index.astro         Chinese landing
src/content/landing.ts           all landing copy, both languages
src/layouts/Landing.astro        head, nav, sections, footer, copy button
src/components/landing/          brand mark and agent logo wall
src/content/agent-logos.ts       local SVG paths for the agent logo wall
src/content/docs/docs/           English docs → /docs/**
src/content/docs/zh/docs/        Chinese docs → /zh/docs/**
src/content/docs/404?            Starlight's 404
src/components/Head.astro        TechArticle + BreadcrumbList for docs
src/sidebar.mjs                  sidebar labels and translations
src/styles/tokens.css            the porcelain tokens (one source)
src/styles/landing.css           the marketing page
src/styles/docs.css              Starlight token overrides
public/                          favicons, OG cards, robots.txt, _headers
public/screens/                  live device screens + sanitized console UI shots
```

### Content rules

- Every landing string lives in `src/content/landing.ts`; the `zh` object is typed
  against `en`, so a missing field fails the build.
- The agent logos are local SVG data in `src/content/agent-logos.ts`, extracted
  once from the `@thesvg/react` dataset. There is no runtime icon dependency and
  no external request. Keep the trademark line if the logo set changes.
- Docs links are root-absolute and locale-specific: `/docs/...` in English,
  `/zh/docs/...` in Chinese. `scripts/check-links.mjs` resolves both paths and
  checks the target file and anchor after every build.
- A new docs page needs an entry in `src/sidebar.mjs` if it should appear in the
  sidebar. `translations` keys are BCP-47 language tags (`zh-CN`), not locale
  keys (`zh`).
- The hub has one theme. The docs force the light scheme and hide Starlight's
  theme selector; do not add a dark mode just to the site.
- Do not put the marketing site into the Go binary. The hub's `/` is its login
  page; this site is a separate deliverable.

## SEO

The build emits:

- `sitemap-index.xml` and `sitemap-0.xml` with every page
- canonical, `hreflang` (`en`, `zh-CN`, `x-default`), Open Graph and Twitter tags
- `SoftwareApplication` JSON-LD on the landing pages
- `TechArticle` + `BreadcrumbList` JSON-LD on every docs page
- `llms.txt`, `llms-small.txt`, `llms-full.txt` via `starlight-llms-txt`
- static Pagefind search (no server)
- `robots.txt` and `_headers` (Cloudflare Pages format; Caddy ignores the latter)

The OG cards are committed. Regenerate them with `npm run og` after changing the
hero or the brand mark.

The screenshots are from a real hub. The console UI shots in the docs have
device names, addresses, and the hub URL replaced by examples. The landing
carousel uses five live framebuffer crops (`/screens/live-*.webp`) with the hub
chrome and the device rail cropped out, so no hostname or token is visible. The
docs home uses 320/160px `-thumb.webp` variants of the same files as a contact
sheet. The docs stylesheet caps UI shots at 640px so the text stays legible.

If the console or the device set changes, recapture and replace the files. Keep
one desktop and one phone screen per platform, keep the file names, and do not
add a screenshot that contains an admin token, a real hostname, or a private IP.

## Deploy

The build output is `dist/`. Any static host works.

The production setup is a Caddy container on the origin. The build is synced to
`/data/site` inside Caddy's data mount; Caddy serves `abacad.ai`, redirects
`www` and `next` to the apex, and returns `410` for the retired relay paths.
`_headers` only applies on Cloudflare Pages; with Caddy, the cache and security
headers live in the Caddyfile (and `_headers` is blocked from being served).

The previous hosted relay at the apex has been retired. Its `/login`, `/pair`,
`/devices`, and random `*.abacad.ai` pages are not part of this site and do not
get redirects. The SSH jump host is a separate service and is unaffected by the
web cutover.
