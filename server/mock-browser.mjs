// Stand-in for a *browser* device, for verifying the server end-to-end without a
// real browser. Connects to the /device WebSocket and answers the browser
// surface: the semantic verbs (screenshot / click / scroll / input_text) plus
// `execute`, which really does run the supplied code as an async function body
// (in Node here, not a DOM) so the JSON return-value round-trip is exercised for
// real. A thrown error comes back as a failed reply, exactly like the browser
// client's async-eval will.
//
// Run: SERVER_URL="ws://localhost:8848/device?token=<device-token>" node mock-browser.mjs
import WebSocket from "ws";

const url = process.env.SERVER_URL ?? "ws://localhost:8848/device";

// A real 1x1 PNG (so the MCP image content block is a valid image).
const PNG_1x1 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC";

function connect() {
  const ws = new WebSocket(url);

  ws.on("open", () => console.error("[browser] connected to", url));

  ws.on("message", async (data) => {
    let cmd;
    try {
      cmd = JSON.parse(data.toString());
    } catch {
      return;
    }
    let result;
    switch (cmd.method) {
      case "screenshot":
        // The "surface" is a tiny fake page; the DOM-derived tree mirrors what the
        // real browser client will build from getBoundingClientRect + roles.
        result = { w: 800, h: 600, png_base64: PNG_1x1 };
        if (cmd.params?.include_ui_tree !== false) {
          result.tree = {
            pkg: "https://demo.abacad.ai/",
            nodes: [
              { cls: "H1", text: "Demo surface", id: "", clickable: false, bounds: [24, 24, 400, 64] },
              { cls: "INPUT", text: "", id: "q", clickable: true, bounds: [24, 96, 360, 132] },
              { cls: "BUTTON", text: "Go", id: "go", clickable: true, bounds: [376, 96, 456, 132] },
            ],
          };
        }
        break;
      // click(=tap), scroll, swipe all resolve to a dispatched gesture in-page.
      case "tap":
      case "click":
      case "long_press":
      case "swipe":
      case "scroll":
        result = { dispatched: true };
        break;
      case "input_text":
        result = { set: true };
        break;
      case "execute": {
        const code = cmd.params?.code ?? "";
        try {
          // Run as an async function body, so `return <value>` and `await` work —
          // the same contract the real client honors via the iframe realm.
          const fn = new Function(`return (async () => { ${code} })()`);
          const value = await fn();
          result = { value: value === undefined ? null : value };
        } catch (e) {
          ws.send(JSON.stringify({ id: cmd.id, ok: false, error: String((e && e.message) || e) }));
          console.error("[browser] execute threw:", (e && e.message) || e);
          return;
        }
        break;
      }
      default:
        // Nav keys (back/home/recents) and desktop-only verbs are not implemented
        // on a browser device — reject as unknown, the framework's contract.
        ws.send(JSON.stringify({ id: cmd.id, ok: false, error: `unknown method ${cmd.method}` }));
        return;
    }
    ws.send(JSON.stringify({ id: cmd.id, ok: true, result }));
    console.error("[browser] handled", cmd.method);
  });

  ws.on("close", () => {
    console.error("[browser] closed; reconnecting in 500ms");
    setTimeout(connect, 500);
  });
  ws.on("error", (e) => console.error("[browser] error:", e.message));
}

connect();
