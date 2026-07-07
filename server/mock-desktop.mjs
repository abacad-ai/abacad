// Stand-in for a *desktop* device (mac mini / Linux box), for verifying the
// tunnel end-to-end without a real backend. Connects to the /device WebSocket
// and speaks only the binary tunnel lane: on a StreamOpen frame it dials the
// target TCP host:port and pipes bytes both ways. It ignores the JSON command
// lane (that is the Android/accessibility protocol, not a desktop's job).
//
// Run: SERVER_URL="ws://localhost:8848/device?token=<device-token>" node mock-desktop.mjs
import WebSocket from "ws";
import net from "node:net";

const url = process.env.SERVER_URL ?? "ws://localhost:8848/device";

const OPEN = 1, DATA = 2, CLOSE = 3;

function encode(type, id, payload = Buffer.alloc(0)) {
  const head = Buffer.alloc(9);
  head[0] = type;
  head.writeBigUInt64BE(BigInt(id), 1);
  return Buffer.concat([head, Buffer.isBuffer(payload) ? payload : Buffer.from(payload)]);
}

function splitHostPort(hp) {
  const i = hp.lastIndexOf(":");
  return [hp.slice(0, i), Number(hp.slice(i + 1))];
}

function connect() {
  const ws = new WebSocket(url);
  ws.binaryType = "nodebuffer";
  const socks = new Map(); // stream id -> TCP socket

  ws.on("open", () => console.error("[desktop] connected to", url));

  ws.on("message", (data, isBinary) => {
    if (!isBinary) return; // command lane (accessibility JSON) — not supported here
    const type = data[0];
    const id = data.readBigUInt64BE(1).toString();
    const payload = data.subarray(9);

    if (type === OPEN) {
      const [host, port] = splitHostPort(payload.toString());
      const sock = net.connect(port, host, () =>
        console.error("[desktop] stream", id, "-> dialed", host + ":" + port));
      socks.set(id, sock);
      sock.on("data", (b) => ws.send(encode(DATA, id, b)));
      sock.on("close", () => {
        if (socks.delete(id)) ws.send(encode(CLOSE, id));
      });
      sock.on("error", (e) => {
        if (socks.delete(id)) ws.send(encode(CLOSE, id, Buffer.from(String(e.message))));
      });
    } else if (type === DATA) {
      const sock = socks.get(id);
      if (sock) sock.write(payload);
    } else if (type === CLOSE) {
      const sock = socks.get(id);
      if (sock) { socks.delete(id); sock.destroy(); }
    }
  });

  ws.on("close", () => {
    for (const s of socks.values()) s.destroy();
    socks.clear();
    console.error("[desktop] closed; reconnecting in 500ms");
    setTimeout(connect, 500);
  });
  ws.on("error", (e) => console.error("[desktop] error:", e.message));
}

connect();
