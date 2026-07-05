import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { deviceHub } from "./device.js";
import type { ScreenshotResult, SleepResult, TapResult, UiTreeResult, WakeResult } from "./protocol.js";

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

  server.registerTool(
    "swipe",
    {
      description:
        "Swipe/drag on the connected Android device from (x1,y1) to (x2,y2) over duration_ms (default 300). Use for scrolling and navigation — e.g. to advance a vertical video feed, swipe from a lower point to a higher point (bottom -> top); a shorter duration flings faster. Absolute pixels; get screen size from a screenshot.",
      inputSchema: {
        x1: z.number().int().describe("start x pixel"),
        y1: z.number().int().describe("start y pixel"),
        x2: z.number().int().describe("end x pixel"),
        y2: z.number().int().describe("end y pixel"),
        duration_ms: z.number().int().optional().describe("gesture duration in ms (default 300)"),
      },
    },
    ({ x1, y1, x2, y2, duration_ms }) =>
      runTool(async () => {
        const dur = duration_ms ?? 300;
        const r = (await deviceHub.send("swipe", { x1, y1, x2, y2, duration_ms: dur })) as { dispatched: boolean };
        return { content: [{ type: "text", text: `swipe dispatched=${r.dispatched} (${x1},${y1})->(${x2},${y2}) ${dur}ms` }] };
      }),
  );

  server.registerTool(
    "wake",
    {
      description:
        "Wake the connected Android device: turn the screen on and dismiss a non-secure (swipe/none) keyguard so ui_tree/screenshot/tap work. Call this FIRST when the device may be idle with the screen off. Returns whether the screen is on, whether the keyguard is secure, and whether it is now unlocked. If keyguard_secure is true the device has a PIN/pattern and CANNOT be auto-unlocked — a human must unlock it once.",
      inputSchema: {},
    },
    () =>
      runTool(async () => {
        const r = (await deviceHub.send("wake")) as WakeResult;
        return { content: [{ type: "text", text: JSON.stringify(r, null, 2) }] };
      }),
  );

  server.registerTool(
    "sleep",
    {
      description:
        "Put the connected Android device back to sleep (turn the screen off) between tasks to save power. Requires the one-time device-admin grant in the app; if it isn't granted this returns an error and the screen just follows its normal timeout instead.",
      inputSchema: {},
    },
    () =>
      runTool(async () => {
        const r = (await deviceHub.send("sleep")) as SleepResult;
        return { content: [{ type: "text", text: `sleep locked=${r.locked}` }] };
      }),
  );

  return server;
}
