// Live verification of the REAL browser client (server/backend/internal/web/browser.html)
// in a headless Chromium. Playwright opens /b#<device-token> so the actual client
// connects to the relay as a device; then an MCP client drives it as an agent
// would. This exercises the parts the mock can't: html2canvas capture, the
// DOM-derived tree, synthetic-event injection, and execute running in the iframe realm.
//
//   MCP_URL=... MCP_TOKEN=... DEVICE_TOKEN=... BASE=... node verify-browser-live.mjs
import { chromium } from "playwright";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

// DEVICE_URL is the device's own subdomain page, e.g. http://<id>.localhost:PORT/
// (Chromium resolves *.localhost to loopback, and the Host carries the id — the
// same routing prod uses for <id>.abacad.ai). The agent side talks to BASE/mcp.
const BASE = process.env.BASE ?? "http://localhost:8848";
const DEVICE_URL = process.env.DEVICE_URL ?? `${BASE}/`;
const MCP_TOKEN = process.env.MCP_TOKEN ?? "";

const checks = [];
const check = (name, ok, extra = "") => { checks.push({ name, ok }); console.log(`${ok ? "ok  " : "FAIL"}  ${name}${extra ? "  — " + extra : ""}`); };
const textOf = (r) => r.content.filter((c) => c.type === "text").map((c) => c.text).join("\n");

// --- launch the real client in a headless browser ---
const browser = await chromium.launch({ headless: true, args: ["--no-sandbox", "--disable-setuid-sandbox"] });
const page = await browser.newPage({ viewport: { width: 900, height: 640 } });
const consoleErrs = [];
page.on("console", (m) => { if (m.type() === "error") consoleErrs.push(m.text()); });
page.on("pageerror", (e) => consoleErrs.push(String(e)));
// html2canvas must load same-origin (vendored), never from a CDN.
const cdnHits = [], hcHits = [];
page.on("request", (req) => {
  const u = req.url();
  if (/jsdelivr|unpkg|cdnjs|googleapis/.test(u)) cdnHits.push(u);
  if (/\/_hc\.js(\?|$)/.test(u)) hcHits.push(u);
});

await page.goto(DEVICE_URL, { waitUntil: "load" });
// The dot turns green when the relay socket opens — i.e. the Host-based auth
// (subdomain id -> device) succeeded with no token in the URL.
await page.waitForSelector("#dot.on", { timeout: 10000 }).then(() => check("client connects via subdomain (Host-auth, no token)", true))
  .catch(() => check("client connects via subdomain (Host-auth, no token)", false));
await page.waitForTimeout(400); // let the default srcdoc surface render

// --- drive it as an agent over MCP ---
const client = new Client({ name: "abacad-verify-live", version: "0.0.0" });
await client.connect(new StreamableHTTPClientTransport(new URL(`${BASE}/mcp`),
  { requestInit: { headers: { Authorization: `Bearer ${MCP_TOKEN}` } } }));

const call = (name, args) => client.callTool({ name, arguments: args });

// list_devices sees it online
const list = textOf(await call("list_devices", {}));
check("list_devices shows an online device", /online/i.test(list) || /true/.test(list), list.slice(0, 120));

// execute in the iframe realm: arithmetic, then READ the surface's real DOM
const execNum = textOf(await call("execute", { code: "return 6 * 7" }));
check("execute evaluates JS (6*7=42)", /\b42\b/.test(execNum), execNum);

const execRead = textOf(await call("execute", { code: "return document.querySelector('h1')?.innerText || ''" }));
check("execute reads the surface DOM (h1 text)", /abacad browser device/i.test(execRead), execRead);

// execute mutates the surface DOM, then a screenshot's tree must reflect it
const execMut = textOf(await call("execute", {
  code: "document.body.innerHTML = '<h1>Live</h1><button id=go>Go</button>'; return document.querySelectorAll('button').length",
}));
check("execute mutates the surface DOM (1 button)", /\b1\b/.test(execMut), execMut);

const shot = await call("screenshot", {});
const shotText = textOf(shot);
const hasImage = shot.content.some((c) => c.type === "image" && typeof c.data === "string" && c.data.length > 0);
check("screenshot returns a DOM tree", /"nodes"/.test(shotText));
check("tree reflects the mutation (button id=go)", /"go"/.test(shotText) && /Go/.test(shotText), "");
check("screenshot returns pixels (html2canvas)", hasImage, hasImage ? "" : "empty image — html2canvas may be unreachable/failed");
check("html2canvas served same-origin (vendored, no CDN)", cdnHits.length === 0 && hcHits.length > 0,
  `cdn=${cdnHits.length} /_hc.js=${hcHits.length}`);

// click by the button's bounds; verify by wiring a flag the click should set
await call("execute", { code: "window.__clicked=false; document.getElementById('go').addEventListener('click',()=>window.__clicked=true);" });
// find the button center from the screenshot's tree (parse the JSON content block)
function parseTree(r) {
  for (const c of r.content) {
    if (c.type === "text") { try { const o = JSON.parse(c.text); if (o && Array.isArray(o.nodes)) return o; } catch { /* not the tree block */ } }
  }
  return null;
}
let cx = 40, cy = 40;
const tree = parseTree(shot);
const go = tree && tree.nodes.find((n) => n.id === "go");
if (go) { cx = Math.round((go.bounds[0] + go.bounds[2]) / 2); cy = Math.round((go.bounds[1] + go.bounds[3]) / 2); }
const clickRes = textOf(await call("click", { x: cx, y: cy }));
const clicked = textOf(await call("execute", { code: "return window.__clicked === true" }));
check("click injects a real DOM event", /dispatched=true/.test(clickRes) && /true/.test(clicked), `at (${cx},${cy})`);

// Navigate the surface to an opaque-origin document (a data: URL — no network,
// deterministically cross-origin) and verify execute degrades to a clear
// look-only error rather than crashing.
await call("execute", { code: "location.href='data:text/html,<h1>opaque</h1>'" });
await page.waitForTimeout(500);
const afterNav = await call("execute", { code: "return 1" });
check("cross-origin surface → execute reports look-only (no crash)",
  afterNav.isError === true && /cross-origin/i.test(textOf(afterNav)), textOf(afterNav).slice(0, 90));

await client.close();
await browser.close();

if (consoleErrs.length) console.log("browser console errors:\n  " + consoleErrs.join("\n  "));
const passed = checks.filter((c) => c.ok).length;
const failed = checks.length - passed;
console.log(`\n${passed}/${checks.length} checks passed`);
process.exit(failed === 0 ? 0 : 1);
