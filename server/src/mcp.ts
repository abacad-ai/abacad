import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { deviceHub } from "./device.js";
import type { ScreenshotResult, TapResult, UiTreeResult } from "./protocol.js";

type ToolResult = {
  content: Array<
    | { type: "text"; text: string }
    | { type: "image"; data: string; mimeType: string }
  >;
  isError?: boolean;
};

// Turn device errors (no device, timeout) into a clean agent-facing message
// instead of an exception.
async function runTool(fn: () => Promise<ToolResult>): Promise<ToolResult> {
  try {
    return await fn();
  } catch (e) {
    return { content: [{ type: "text", text: `Error: ${(e as Error).message}` }], isError: true };
  }
}

export function buildMcpServer(): McpServer {
  const server = new McpServer({ name: "abacad", version: "0.1.0" });

  server.registerTool(
    "ui_tree",
    {
      description:
        "Read the current on-screen UI of the connected Android device as a structured accessibility tree. Returns JSON: the foreground package and a list of nodes, each with class, text, resource id, a clickable flag, and screen bounds [left, top, right, bottom]. Prefer this over screenshot to decide what to interact with; tap the center of a node's bounds.",
      inputSchema: {},
    },
    () =>
      runTool(async () => {
        const r = (await deviceHub.send("ui_tree")) as UiTreeResult;
        return { content: [{ type: "text", text: JSON.stringify(r, null, 2) }] };
      }),
  );

  server.registerTool(
    "screenshot",
    {
      description:
        "Capture the current screen of the connected Android device as a PNG image. Use when the UI tree is empty or insufficient (e.g. canvas/game screens). Note: FLAG_SECURE screens (some banking/payment views) may return black.",
      inputSchema: {},
    },
    () =>
      runTool(async () => {
        const r = (await deviceHub.send("screenshot")) as ScreenshotResult;
        return {
          content: [
            { type: "image", data: r.png_base64, mimeType: "image/png" },
            { type: "text", text: `screen ${r.w}x${r.h}` },
          ],
        };
      }),
  );

  server.registerTool(
    "tap",
    {
      description:
        "Tap the connected Android device screen at absolute pixel coordinates. Get coordinates from ui_tree node bounds — tap the center of the target node.",
      inputSchema: {
        x: z.number().int().describe("x pixel coordinate"),
        y: z.number().int().describe("y pixel coordinate"),
      },
    },
    ({ x, y }) =>
      runTool(async () => {
        const r = (await deviceHub.send("tap", { x, y })) as TapResult;
        return { content: [{ type: "text", text: `tap dispatched=${r.dispatched} at (${x}, ${y})` }] };
      }),
  );

  return server;
}
