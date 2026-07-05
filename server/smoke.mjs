// End-to-end smoke test: acts as the AGENT (MCP client), connects to the server
// over Streamable HTTP, lists tools, and calls each one. With mock-device.mjs
// connected, this proves the full loop: agent -> MCP -> relay -> device -> back.
// Run: node smoke.mjs   (set MCP_URL to override)
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

const url = new URL(process.env.MCP_URL ?? "http://localhost:8848/mcp");

const client = new Client({ name: "abacad-smoke", version: "0.0.0" });
await client.connect(new StreamableHTTPClientTransport(url));

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

const tree = await callWithRetry("ui_tree", {});
const treeText = tree.content.find((c) => c.type === "text")?.text ?? "";
console.log("ui_tree isError:", tree.isError, "| nodes present:", /"nodes"/.test(treeText));

const shot = await client.callTool({ name: "screenshot", arguments: {} });
console.log("screenshot content types:", shot.content.map((c) => c.type).join(","));
const hasImage = shot.content.some((c) => c.type === "image" && typeof c.data === "string" && c.data.length > 0);

const tap = await client.callTool({ name: "tap", arguments: { x: 120, y: 130 } });
console.log("tap:", tap.content.find((c) => c.type === "text")?.text);

const swipe = await client.callTool({ name: "swipe", arguments: { x1: 540, y1: 1400, x2: 540, y2: 400 } });
console.log("swipe:", swipe.content.find((c) => c.type === "text")?.text);

await client.close();

const pass =
  /"nodes"/.test(treeText) &&
  hasImage &&
  /dispatched=true/.test(tap.content[0]?.text ?? "") &&
  /dispatched=true/.test(swipe.content[0]?.text ?? "");
console.log(pass ? "SMOKE OK" : "SMOKE FAILED");
process.exit(pass ? 0 : 1);
