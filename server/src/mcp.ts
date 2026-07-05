import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { z } from "zod";
import { deviceHub } from "./device.js";
import type { GestureResult, GlobalActionResult, InputTextResult, ScreenshotResult } from "./protocol.js";

type ToolResult = {
  content: Array<
    | { type: "text"; text: string }
    | { type: "image"; data: string; mimeType: string }
  >;
  isError?: boolean;
};

// Turn device errors (no device, timeout, locked-with-PIN) into a clean
// agent-facing message instead of an exception.
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
    "screenshot",
    {
      description:
        "Look at the connected Android device's screen. Returns a PNG of the current screen and, by default, the accessibility UI tree: the foreground package plus a list of nodes, each with class, text, resource id, a clickable flag, and screen bounds [left, top, right, bottom]. Use the tree to decide what to interact with — tap the center of a node's bounds. Set include_ui_tree=false for canvas/game screens where the tree is empty or noise (you still get the image). The device is woken automatically if its screen was off.",
      inputSchema: {
        include_ui_tree: z
          .boolean()
          .optional()
          .describe("also return the accessibility UI tree (default true)"),
      },
    },
    ({ include_ui_tree }) =>
      runTool(async () => {
        const includeTree = include_ui_tree ?? true;
        const r = (await deviceHub.send("screenshot", { include_ui_tree: includeTree })) as ScreenshotResult;
        const content: ToolResult["content"] = [
          { type: "image", data: r.png_base64, mimeType: "image/png" },
          { type: "text", text: `screen ${r.w}x${r.h}` },
        ];
        if (r.tree) content.push({ type: "text", text: JSON.stringify(r.tree, null, 2) });
        return { content };
      }),
  );

  server.registerTool(
    "tap",
    {
      description:
        "Tap the connected Android device screen at absolute pixel coordinates. Get coordinates from a screenshot's UI tree node bounds — tap the center of the target node.",
      inputSchema: {
        x: z.number().int().describe("x pixel coordinate"),
        y: z.number().int().describe("y pixel coordinate"),
      },
    },
    ({ x, y }) =>
      runTool(async () => {
        const r = (await deviceHub.send("tap", { x, y })) as GestureResult;
        return { content: [{ type: "text", text: `tap dispatched=${r.dispatched} at (${x}, ${y})` }] };
      }),
  );

  server.registerTool(
    "long_press",
    {
      description:
        "Press and hold at absolute pixel coordinates for duration_ms (default 600). Use for context menus, drag handles, and other press-and-hold interactions where a plain tap won't do.",
      inputSchema: {
        x: z.number().int().describe("x pixel coordinate"),
        y: z.number().int().describe("y pixel coordinate"),
        duration_ms: z.number().int().optional().describe("hold duration in ms (default 600)"),
      },
    },
    ({ x, y, duration_ms }) =>
      runTool(async () => {
        const dur = duration_ms ?? 600;
        const r = (await deviceHub.send("long_press", { x, y, duration_ms: dur })) as GestureResult;
        return { content: [{ type: "text", text: `long_press dispatched=${r.dispatched} at (${x}, ${y}) ${dur}ms` }] };
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
        const r = (await deviceHub.send("swipe", { x1, y1, x2, y2, duration_ms: dur })) as GestureResult;
        return { content: [{ type: "text", text: `swipe dispatched=${r.dispatched} (${x1},${y1})->(${x2},${y2}) ${dur}ms` }] };
      }),
  );

  server.registerTool(
    "input_text",
    {
      description:
        "Type text into the currently focused input field on the connected Android device. Tap the field first to focus it, then call this. Replaces the field's current contents. For submitting/searching, follow with the on-screen action button (e.g. tap the keyboard's Enter/Search key via its node).",
      inputSchema: {
        text: z.string().describe("text to place into the focused field"),
      },
    },
    ({ text }) =>
      runTool(async () => {
        const r = (await deviceHub.send("input_text", { text })) as InputTextResult;
        return { content: [{ type: "text", text: `input_text set=${r.set}` }] };
      }),
  );

  const globalAction = (name: "back" | "home" | "recents", description: string) =>
    server.registerTool(name, { description, inputSchema: {} }, () =>
      runTool(async () => {
        const r = (await deviceHub.send(name)) as GlobalActionResult;
        return { content: [{ type: "text", text: `${name} performed=${r.performed}` }] };
      }),
    );

  globalAction("back", "Press the Android Back navigation key: go back one step / dismiss the current screen or keyboard.");
  globalAction("home", "Press the Android Home navigation key: go to the launcher home screen.");
  globalAction("recents", "Press the Android Recents (overview) navigation key: open the recent-apps switcher.");

  return server;
}
