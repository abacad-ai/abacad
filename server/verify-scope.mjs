// End-to-end check of API-key scope enforcement. Registers an account, creates
// two devices and a narrowly-scoped key (device1 + screenshot only, no tunnel),
// connects a mock device, and asserts every gate. Then widens the key and
// re-checks. Run against a fresh server on :8899.
import WebSocket from "ws";

const BASE = "http://localhost:8899";
let pass = 0,
  fail = 0;
function check(name, ok, extra = "") {
  (ok ? (pass++, console.log(`  ok   ${name}`)) : (fail++, console.log(`  FAIL ${name} ${extra}`)));
}

async function api(path, { method = "GET", body, cookie, bearer } = {}) {
  const headers = { "Content-Type": "application/json" };
  if (cookie) headers.Cookie = cookie;
  if (bearer) headers.Authorization = "Bearer " + bearer;
  const res = await fetch(BASE + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
  const setCookie = res.headers.get("set-cookie");
  const text = await res.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    /* non-JSON */
  }
  return { status: res.status, json, text, cookie: setCookie ? setCookie.split(";")[0] : null };
}

async function rpc(bearer, method, params) {
  const res = await fetch(BASE + "/mcp", {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: "Bearer " + bearer },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
  });
  return { status: res.status, json: await res.json() };
}
const isErr = (r) => r.json?.result?.isError === true;
const textOf = (r) => (r.json?.result?.content ?? []).map((c) => c.text ?? `[${c.type}]`).join(" ");

async function main() {
  const email = `t${Date.now()}@x.com`;
  const reg = await api("/api/auth/register", { method: "POST", body: { email, password: "secret1" } });
  const cookie = reg.cookie;
  check("register", reg.status === 201 && cookie, JSON.stringify(reg.json));

  const d1 = await api("/api/devices", { method: "POST", body: { name: "Phone" }, cookie });
  const d2 = await api("/api/devices", { method: "POST", body: { name: "Mac", platform: "macos" }, cookie });
  const id1 = d1.json.id,
    id2 = d2.json.id,
    tok1 = d1.json.device_token;
  check("two devices created", id1 && id2 && tok1);

  // Narrow key: device1 + screenshot only, no tunnel.
  const mk = await api("/api/keys", {
    method: "POST",
    cookie,
    body: { name: "narrow", all_devices: false, device_ids: [id1], all_methods: false, methods: ["screenshot"], allow_tunnel: false },
  });
  const secret = mk.json.secret,
    keyId = mk.json.key.id;
  check("scoped key created (secret shown once)", mk.status === 201 && secret?.startsWith("abd_key_") && keyId, JSON.stringify(mk.json));

  // Connect a mock device for device1.
  const ws = new WebSocket(`ws://localhost:8899/device?token=${tok1}`);
  await new Promise((res, rej) => {
    ws.on("open", res);
    ws.on("error", rej);
  });
  ws.on("message", (data) => {
    const cmd = JSON.parse(data.toString());
    if (cmd.method === "screenshot") {
      ws.send(JSON.stringify({ id: cmd.id, ok: true, result: { w: 1, h: 1, png_base64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC" } }));
    } else {
      ws.send(JSON.stringify({ id: cmd.id, ok: true, result: { dispatched: true } }));
    }
  });
  await new Promise((r) => setTimeout(r, 300)); // let the device register

  console.log("\n[narrow key]");
  const list = await rpc(secret, "tools/list", {});
  const names = (list.json.result?.tools ?? []).map((t) => t.name);
  check("tools/list has screenshot", names.includes("screenshot"));
  check("tools/list has list_devices", names.includes("list_devices"));
  check("tools/list hides tap (not in scope)", !names.includes("tap"), JSON.stringify(names));

  const ld = await rpc(secret, "tools/call", { name: "list_devices", arguments: {} });
  check("list_devices shows device1", textOf(ld).includes(id1));
  check("list_devices hides device2", !textOf(ld).includes(id2), textOf(ld));

  const shot = await rpc(secret, "tools/call", { name: "screenshot", arguments: { device_id: id1 } });
  check("screenshot allowed on in-scope device", !isErr(shot), textOf(shot));

  const tap = await rpc(secret, "tools/call", { name: "tap", arguments: { device_id: id1, x: 1, y: 1 } });
  check("tap denied (method not in scope)", isErr(tap) && /not permitted/.test(textOf(tap)), textOf(tap));

  const shot2 = await rpc(secret, "tools/call", { name: "screenshot", arguments: { device_id: id2 } });
  check("screenshot on out-of-scope device denied", isErr(shot2) && /not permitted/.test(textOf(shot2)), textOf(shot2));

  const conn = await api(`/connect?token=${secret}&device=${id1}&target=127.0.0.1:22`);
  check("connect tunnel denied (403)", conn.status === 403, `status=${conn.status} ${conn.text}`);

  // Widen: all devices + all methods + tunnel.
  console.log("\n[widened key]");
  const upd = await api(`/api/keys/${keyId}`, {
    method: "PATCH",
    cookie,
    body: { name: "wide", all_devices: true, all_methods: true, methods: [], device_ids: [], allow_tunnel: true },
  });
  check("update to all/all/tunnel", upd.status === 204, `status=${upd.status}`);

  const list2 = await rpc(secret, "tools/list", {});
  const names2 = (list2.json.result?.tools ?? []).map((t) => t.name);
  check("tools/list now includes tap", names2.includes("tap"));

  const tap2 = await rpc(secret, "tools/call", { name: "tap", arguments: { device_id: id1, x: 1, y: 1 } });
  check("tap now allowed", !isErr(tap2), textOf(tap2));

  const ld2 = await rpc(secret, "tools/call", { name: "list_devices", arguments: {} });
  check("list_devices now shows device2 too", textOf(ld2).includes(id2));

  const conn2 = await api(`/connect?token=${secret}&device=${id1}&target=127.0.0.1:22`);
  check("connect tunnel gate passes (not 403)", conn2.status !== 403, `status=${conn2.status}`);

  ws.close();
  console.log(`\n${fail === 0 ? "SCOPE OK" : "SCOPE FAILED"} — ${pass} passed, ${fail} failed`);
  process.exit(fail === 0 ? 0 : 1);
}
main().catch((e) => {
  console.error(e);
  process.exit(1);
});
