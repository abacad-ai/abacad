// End-to-end tunnel test: starts a local TCP echo server, opens the /connect
// tunnel to it (through the relay and mock-desktop), sends bytes, and asserts
// they come back byte-identical. Also checks a refused-connection surfaces as a
// close rather than a hang.
//
// Prereqs (started by run-tunnel-test.sh):
//   - the Go server with -seed
//   - mock-desktop.mjs connected with the seed device token
// Env: ABACAD_TOKEN=<api-key>  [ABACAD_WS=ws://localhost:8848] [ABACAD_DEVICE=<id>]
import net from "node:net";
import WebSocket from "ws";

const server = process.env.ABACAD_WS ?? "ws://localhost:8848";
const token = process.env.ABACAD_TOKEN;
const device = process.env.ABACAD_DEVICE ?? ""; // empty -> account default (the online mock-desktop)

if (!token) { console.error("set ABACAD_TOKEN"); process.exit(2); }

function tunnelURL(target) {
  return `${server}/connect?token=${encodeURIComponent(token)}` +
    `&device=${encodeURIComponent(device)}&target=${encodeURIComponent(target)}`;
}

const fail = (msg) => { console.error("FAIL:", msg); process.exit(1); };

async function testEcho() {
  const echo = net.createServer((s) => s.pipe(s));
  await new Promise((r) => echo.listen(0, "127.0.0.1", r));
  const port = echo.address().port;

  const payload = Buffer.from("hello-abacad-tunnel-" + "x".repeat(5000)); // > one frame chunk? no, but multi-write
  const ws = new WebSocket(tunnelURL(`127.0.0.1:${port}`));
  ws.binaryType = "nodebuffer";

  const got = [];
  await new Promise((resolve) => {
    const timer = setTimeout(() => fail("echo timed out"), 5000);
    ws.on("open", () => ws.send(payload));
    ws.on("message", (d) => {
      got.push(Buffer.from(d));
      const total = Buffer.concat(got);
      if (total.length >= payload.length) {
        clearTimeout(timer);
        if (!total.equals(payload)) fail(`echo mismatch: got ${total.length} bytes`);
        ws.close();
        echo.close();
        console.log("OK  echo round-trip", payload.length, "bytes");
        resolve();
      }
    });
    ws.on("error", (e) => fail("echo ws error: " + e.message));
  });
}

async function testRefused() {
  // Nothing listens on this port -> device dial fails -> we expect a prompt close.
  const ws = new WebSocket(tunnelURL("127.0.0.1:1"));
  ws.binaryType = "nodebuffer";
  await new Promise((resolve) => {
    const timer = setTimeout(() => fail("refused case hung (no close)"), 5000);
    ws.on("open", () => ws.send(Buffer.from("ping")));
    ws.on("close", () => { clearTimeout(timer); console.log("OK  refused target closed the stream"); resolve(); });
    ws.on("error", () => { clearTimeout(timer); console.log("OK  refused target errored the stream"); resolve(); });
  });
}

await testEcho();
await testRefused();
console.log("ALL TUNNEL TESTS PASSED");
process.exit(0);
