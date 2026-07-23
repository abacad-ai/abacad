// End-to-end smoke test: acts as the AGENT (MCP client), connects to the server
// over Streamable HTTP, lists tools, and calls each one. With a mock device
// connected, this proves the full loop: agent -> MCP -> relay -> device -> back.
//
// The Go server requires auth. Provide the account's MCP token so this client
// can authenticate; the mock device must be connected with that account's device
// token (see README):
//   node smoke.mjs   (set MCP_URL and MCP_TOKEN to override)
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

const url = new URL(process.env.MCP_URL ?? "http://localhost:8848/mcp");
const token = process.env.MCP_TOKEN ?? "";
const transportOpts = token
  ? { requestInit: { headers: { Authorization: `Bearer ${token}` } } }
  : undefined;

const client = new Client({ name: "abacad-smoke", version: "0.0.0" });
await client.connect(new StreamableHTTPClientTransport(url, transportOpts));

const { tools } = await client.listTools();
console.log("tools:", tools.map((t) => t.name).join(", "));

const textOf = (r) =>
  r.content.filter((c) => c.type === "text").map((c) => c.text).join("\n");

// Every tool now requires an explicit device_id — there is no fallback. The mock
// device may still be connecting, so poll list_devices until one is online and
// use its id for every subsequent call.
async function waitForDeviceID(tries = 20) {
  for (let i = 0; i < tries; i++) {
    const r = await client.callTool({ name: "list_devices", arguments: {} });
    try {
      const online = JSON.parse(textOf(r)).find((d) => d.online);
      if (online) return online.device_id;
    } catch {}
    await new Promise((res) => setTimeout(res, 300));
  }
  throw new Error("no device came online");
}
const device_id = await waitForDeviceID();
console.log("device:", device_id);

// screenshot returns an image + (by default) the UI tree.
const shot = await client.callTool({ name: "screenshot", arguments: { device_id } });
const shotText = textOf(shot);
const hasImage = shot.content.some((c) => c.type === "image" && typeof c.data === "string" && c.data.length > 0);
console.log("screenshot content types:", shot.content.map((c) => c.type).join(","), "| tree present:", /"nodes"/.test(shotText));

// opting out of the tree still yields an image, no tree.
const shotNoTree = await client.callTool({ name: "screenshot", arguments: { device_id, include_ui_tree: false } });
const treeSuppressed = !/"nodes"/.test(textOf(shotNoTree));

const tap = await client.callTool({ name: "tap", arguments: { device_id, x: 120, y: 130 } });
console.log("tap:", textOf(tap));

const longPress = await client.callTool({ name: "long_press", arguments: { device_id, x: 120, y: 130 } });
console.log("long_press:", textOf(longPress));

const swipe = await client.callTool({ name: "swipe", arguments: { device_id, x1: 540, y1: 1400, x2: 540, y2: 400 } });
console.log("swipe:", textOf(swipe));

const type = await client.callTool({ name: "input_text", arguments: { device_id, text: "hello world" } });
console.log("input_text:", textOf(type));

const back = await client.callTool({ name: "back", arguments: { device_id } });
const home = await client.callTool({ name: "home", arguments: { device_id } });
const recents = await client.callTool({ name: "recents", arguments: { device_id } });
console.log("nav keys:", textOf(back), "|", textOf(home), "|", textOf(recents));

await client.close();

const pass =
  hasImage &&
  /"nodes"/.test(shotText) &&
  treeSuppressed &&
  /dispatched=true/.test(textOf(tap)) &&
  /dispatched=true/.test(textOf(longPress)) &&
  /dispatched=true/.test(textOf(swipe)) &&
  /set=true/.test(textOf(type)) &&
  /performed=true/.test(textOf(back)) &&
  /performed=true/.test(textOf(home)) &&
  /performed=true/.test(textOf(recents));
console.log(pass ? "SMOKE OK" : "SMOKE FAILED");
process.exit(pass ? 0 : 1);
