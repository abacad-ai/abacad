// Multi-tenant end-to-end test. Provisions two accounts through the dashboard
// API, connects a mock device for each, then drives two MCP clients (one bearer
// per account) and asserts: device isolation, correct routing, cross-account
// denial, and 401 without a bearer.
//
// Run:  BASE=http://localhost:8905 node test-multi.mjs
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
import WebSocket from "ws";

const BASE = process.env.BASE ?? "http://localhost:8905";
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const textOf = (r) => r.content.filter((c) => c.type === "text").map((c) => c.text).join("\n");

let failures = 0;
function check(cond, label) {
  console.log(`${cond ? "ok  " : "FAIL"}  ${label}`);
  if (!cond) failures++;
}

async function api(path, { method = "GET", body, cookie } = {}) {
  const res = await fetch(BASE + path, {
    method,
    headers: { "Content-Type": "application/json", ...(cookie ? { Cookie: cookie } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  });
  const setCookie = res.headers.get("set-cookie");
  const text = await res.text();
  let json;
  try { json = JSON.parse(text); } catch { /* non-JSON */ }
  return { status: res.status, json, cookie: setCookie ? setCookie.split(";")[0] : null };
}

async function provision(email) {
  const reg = await api("/api/auth/register", { method: "POST", body: { email, password: "secret1" } });
  if (reg.status !== 201) throw new Error(`register ${email}: ${reg.status} ${JSON.stringify(reg.json)}`);
  const cookie = reg.cookie;
  const dev = await api("/api/devices", { method: "POST", body: { name: `Phone-${email}` }, cookie });
  const mcp = await api("/api/mcp-token/rotate", { method: "POST", cookie });
  return { cookie, deviceId: dev.json.id, wssUrl: dev.json.wss_url, mcpToken: mcp.json.mcp_token };
}

const PNG_1x1 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC";
function startMock(wsUrl) {
  const ws = new WebSocket(wsUrl);
  ws.on("message", (d) => {
    const c = JSON.parse(d.toString());
    let r;
    switch (c.method) {
      case "screenshot": r = { w: 1, h: 1, png_base64: PNG_1x1 }; break;
      case "tap": case "long_press": case "swipe": r = { dispatched: true }; break;
      case "input_text": r = { set: true }; break;
      default: r = { performed: true };
    }
    ws.send(JSON.stringify({ id: c.id, ok: true, result: r }));
  });
  return new Promise((res) => ws.on("open", () => res(ws)));
}

function mcpClient(token) {
  const c = new Client({ name: "multi", version: "0" });
  const t = new StreamableHTTPClientTransport(new URL(BASE + "/mcp"), {
    requestInit: { headers: { Authorization: "Bearer " + token } },
  });
  return c.connect(t).then(() => c);
}

const A = await provision("a@test.com");
const B = await provision("b@test.com");

// wss_url is ws:// on a local http server; connect the two mock devices.
const mockA = await startMock(A.wssUrl);
const mockB = await startMock(B.wssUrl);
await sleep(400); // let the hub register both

const ca = await mcpClient(A.mcpToken);
const cb = await mcpClient(B.mcpToken);

// 1. Isolation: each account lists only its own device, shown online.
const la = JSON.parse(textOf(await ca.callTool({ name: "list_devices", arguments: {} })));
const lb = JSON.parse(textOf(await cb.callTool({ name: "list_devices", arguments: {} })));
check(la.length === 1 && la[0].device_id === A.deviceId && la[0].online === true, "A sees exactly its own device, online");
check(lb.length === 1 && lb[0].device_id === B.deviceId && lb[0].online === true, "B sees exactly its own device, online");

// 2. Routing: default screenshot reaches each account's own mock (image back).
const sa = await ca.callTool({ name: "screenshot", arguments: { include_ui_tree: false } });
check(sa.content.some((c) => c.type === "image"), "A default screenshot returns an image");

// 3. Explicit device_id for own device works.
const tapOwn = await ca.callTool({ name: "tap", arguments: { x: 1, y: 1, device_id: A.deviceId } });
check(/dispatched=true/.test(textOf(tapOwn)), "A can target its own device_id");

// 4. Cross-account: A targeting B's device_id is denied, not leaked.
const cross = await ca.callTool({ name: "tap", arguments: { x: 1, y: 1, device_id: B.deviceId } });
check(cross.isError && /not in your account/.test(textOf(cross)), "A cannot target B's device_id");

// 5. No bearer -> 401.
const noauth = await fetch(BASE + "/mcp", {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/list" }),
});
check(noauth.status === 401, "missing bearer is rejected with 401");

// 6. Bogus bearer -> 401.
const badauth = await fetch(BASE + "/mcp", {
  method: "POST",
  headers: { "Content-Type": "application/json", Authorization: "Bearer abd_mcp_nope" },
  body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/list" }),
});
check(badauth.status === 401, "invalid bearer is rejected with 401");

await ca.close();
await cb.close();
mockA.close();
mockB.close();

console.log(failures === 0 ? "MULTI OK" : `MULTI FAILED (${failures})`);
process.exit(failures === 0 ? 0 : 1);
