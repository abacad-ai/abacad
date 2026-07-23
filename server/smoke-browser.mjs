// End-to-end smoke test for the BROWSER device surface: acts as the AGENT (MCP
// client), lists tools, and drives a connected browser mock. Proves the full
// loop for the new `execute` verb plus the semantic shortcuts: agent -> MCP ->
// relay -> browser device -> back.
//
//   node smoke-browser.mjs   (set MCP_URL and MCP_TOKEN to override)
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";

const url = new URL(process.env.MCP_URL ?? "http://localhost:8848/mcp");
const token = process.env.MCP_TOKEN ?? "";
const transportOpts = token
  ? { requestInit: { headers: { Authorization: `Bearer ${token}` } } }
  : undefined;

const client = new Client({ name: "abacad-smoke-browser", version: "0.0.0" });
await client.connect(new StreamableHTTPClientTransport(url, transportOpts));

const { tools } = await client.listTools();
const toolNames = tools.map((t) => t.name);
console.log("tools:", toolNames.join(", "));
const hasExecuteTool = toolNames.includes("execute");

const textOf = (r) =>
  r.content.filter((c) => c.type === "text").map((c) => c.text).join("\n");

// Every tool now requires an explicit device_id — there is no fallback. Poll
// list_devices until the browser mock is online and reuse its id below.
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

// screenshot returns an image + (by default) the DOM tree.
const shot = await client.callTool({ name: "screenshot", arguments: { device_id } });
const shotText = textOf(shot);
const hasImage = shot.content.some((c) => c.type === "image" && typeof c.data === "string" && c.data.length > 0);
console.log("screenshot:", shot.content.map((c) => c.type).join(","), "| tree:", /"nodes"/.test(shotText));

// semantic shortcuts: click (=tap in-page), scroll, input_text.
const click = await client.callTool({ name: "click", arguments: { device_id, x: 416, y: 114 } });
console.log("click:", textOf(click));
const scroll = await client.callTool({ name: "scroll", arguments: { device_id, x: 400, y: 300, dy: 3 } });
console.log("scroll:", textOf(scroll));
const type = await client.callTool({ name: "input_text", arguments: { device_id, text: "hello" } });
console.log("input_text:", textOf(type));

// execute: value round-trip, object serialization, undefined -> "no value",
// and a thrown error surfaced as a tool error.
const execNum = await client.callTool({ name: "execute", arguments: { device_id, code: "return 40 + 2" } });
console.log("execute(number):", textOf(execNum));
const execObj = await client.callTool({ name: "execute", arguments: { device_id, code: "return { msg: 'hi', n: 3 }" } });
console.log("execute(object):", textOf(execObj));
const execVoid = await client.callTool({ name: "execute", arguments: { device_id, code: "globalThis.__x = 1;" } });
console.log("execute(void):", textOf(execVoid));
const execThrow = await client.callTool({ name: "execute", arguments: { device_id, code: "throw new Error('boom')" } });
console.log("execute(throw):", "isError=" + execThrow.isError, "|", textOf(execThrow));

await client.close();

const pass =
  hasExecuteTool &&
  hasImage &&
  /"nodes"/.test(shotText) &&
  /dispatched=true/.test(textOf(click)) &&
  /dispatched=true/.test(textOf(scroll)) &&
  /set=true/.test(textOf(type)) &&
  /(^|\D)42(\D|$)/.test(textOf(execNum)) &&
  /"msg"/.test(textOf(execObj)) && /"hi"/.test(textOf(execObj)) &&
  /no value returned/.test(textOf(execVoid)) &&
  execThrow.isError === true && /boom/.test(textOf(execThrow));
console.log(pass ? "BROWSER SMOKE OK" : "BROWSER SMOKE FAILED");
process.exit(pass ? 0 : 1);
