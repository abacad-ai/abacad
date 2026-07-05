// Stand-in for the Android device, for verifying the server end-to-end without
// a phone. Connects to the /device WebSocket and answers canned commands.
// Run: node mock-device.mjs   (set SERVER_URL to override)
import WebSocket from "ws";

const url = process.env.SERVER_URL ?? "ws://localhost:8848/device";

// A real 1x1 PNG (so the MCP image content block is a valid image).
const PNG_1x1 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC";

function connect() {
  const ws = new WebSocket(url);

  ws.on("open", () => console.error("[mock] connected to", url));

  ws.on("message", (data) => {
    let cmd;
    try {
      cmd = JSON.parse(data.toString());
    } catch {
      return;
    }
    let result;
    switch (cmd.method) {
      case "ui_tree":
        result = {
          pkg: "com.mock.app",
          nodes: [
            { cls: "android.widget.TextView", text: "Hello", id: "", clickable: false, bounds: [40, 40, 300, 80] },
            { cls: "android.widget.Button", text: "OK", id: "com.mock.app:id/ok", clickable: true, bounds: [40, 100, 200, 160] },
          ],
        };
        break;
      case "screenshot":
        result = { w: 1, h: 1, png_base64: PNG_1x1 };
        break;
      case "tap":
      case "swipe":
        result = { dispatched: true };
        break;
      default:
        ws.send(JSON.stringify({ id: cmd.id, ok: false, error: `unknown method ${cmd.method}` }));
        return;
    }
    ws.send(JSON.stringify({ id: cmd.id, ok: true, result }));
    console.error("[mock] handled", cmd.method);
  });

  // Retry so start order (server vs mock) doesn't matter.
  ws.on("close", () => {
    console.error("[mock] closed; reconnecting in 500ms");
    setTimeout(connect, 500);
  });
  ws.on("error", (e) => console.error("[mock] error:", e.message));
}

connect();
