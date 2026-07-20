// abacad SSH ProxyCommand: bridges stdin/stdout to a device-reachable TCP target
// over the /connect tunnel, so the machine behaves as if it were directly
// connected. Wire it into ssh:
//
//   export ABACAD_TOKEN=<api-key>
//   ssh -o ProxyCommand="node ssh-proxy.mjs %h %p" user@<device-id>
//
// %h (the ssh hostname) is used as the abacad device id; %p as the target port
// on that device's own localhost. Set ABACAD_WS to point at a non-local server.
import WebSocket from "ws";

const [device, port, host = "127.0.0.1"] = process.argv.slice(2);
const server = process.env.ABACAD_WS ?? "ws://localhost:8848";
const token = process.env.ABACAD_TOKEN;

if (!device || !port || !token) {
  console.error("usage: ABACAD_TOKEN=<api-key> node ssh-proxy.mjs <device-id> <port> [host]");
  process.exit(2);
}

const url = `${server}/connect?token=${encodeURIComponent(token)}` +
  `&device=${encodeURIComponent(device)}` +
  `&target=${encodeURIComponent(host + ":" + port)}`;

const ws = new WebSocket(url);
ws.binaryType = "nodebuffer";

ws.on("open", () => {
  process.stdin.on("data", (b) => ws.send(b));
  process.stdin.on("end", () => ws.close());
});
ws.on("message", (data) => process.stdout.write(data));
ws.on("close", () => process.exit(0));
ws.on("error", (e) => { console.error("[proxy]", e.message); process.exit(1); });
