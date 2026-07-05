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

// The device may still be connecting; retry the first call briefly.
async function callWithRetry(name, args, tries = 20) {
  for (let i = 0; i < tries; i++) {
    const r = await client.callTool({ name, arguments: args });
    const text = r.content.find((c) => c.type === "text")?.text ?? "";
    if (!r.isError || !/no device connected/.test(text)) return r;
    await new Promise((res) => setTimeout(res, 300));
  }
  throw new Error(`device never connected for ${name}`);
}

const textOf = (r) =>
  r.content.filter((c) => c.type === "text").map((c) => c.text).join("\n");

// screenshot returns an image + (by default) the UI tree.
const shot = await callWithRetry("screenshot", {});
const shotText = textOf(shot);
const hasImage = shot.content.some((c) => c.type === "image" && typeof c.data === "string" && c.data.length > 0);
console.log("screenshot content types:", shot.content.map((c) => c.type).join(","), "| tree present:", /"nodes"/.test(shotText));

// opting out of the tree still yields an image, no tree.
const shotNoTree = await client.callTool({ name: "screenshot", arguments: { include_ui_tree: false } });
const treeSuppressed = !/"nodes"/.test(textOf(shotNoTree));

const tap = await client.callTool({ name: "tap", arguments: { x: 120, y: 130 } });
console.log("tap:", textOf(tap));

const longPress = await client.callTool({ name: "long_press", arguments: { x: 120, y: 130 } });
console.log("long_press:", textOf(longPress));

const swipe = await client.callTool({ name: "swipe", arguments: { x1: 540, y1: 1400, x2: 540, y2: 400 } });
console.log("swipe:", textOf(swipe));

const type = await client.callTool({ name: "input_text", arguments: { text: "hello world" } });
console.log("input_text:", textOf(type));

const back = await client.callTool({ name: "back", arguments: {} });
const home = await client.callTool({ name: "home", arguments: {} });
const recents = await client.callTool({ name: "recents", arguments: {} });
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
