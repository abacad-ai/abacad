import express from "express";
import { createServer } from "node:http";
import { readFileSync } from "node:fs";
import { WebSocketServer } from "ws";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { buildMcpServer } from "./mcp.js";
import { deviceHub } from "./device.js";

// Brand mark, loaded once. SVG is resolution-independent, so the favicon stays
// razor-sharp at every size / DPI — no PNG variants to ship.
const ICON_SVG = readFileSync(new URL("../assets/icon.svg", import.meta.url), "utf8");

// IMPORTANT: this process is an MCP stdio-adjacent server only over HTTP, so
// stdout stays clean regardless; we still log to stderr by convention.
const PORT = Number(process.env.PORT ?? 8848);

const app = express();
app.use(express.json({ limit: "4mb" })); // tool-call inputs are tiny; images flow the other way (over WS + HTTP response)

// MCP endpoint, stateless mode: a fresh server+transport per request. Tools
// close over the shared deviceHub singleton, so no per-session state is needed.
app.post("/mcp", async (req, res) => {
  const server = buildMcpServer();
  const transport = new StreamableHTTPServerTransport({ sessionIdGenerator: undefined });
  res.on("close", () => {
    transport.close();
    server.close();
  });
  try {
    await server.connect(transport);
    await transport.handleRequest(req, res, req.body);
  } catch (e) {
    console.error("[mcp] request error:", e);
    if (!res.headersSent) res.status(500).json({ error: String(e) });
  }
});

// Stateless mode uses POST only.
app.get("/mcp", (_req, res) => {
  res.status(405).json({ error: "method not allowed (stateless MCP: POST only)" });
});
app.delete("/mcp", (_req, res) => {
  res.status(405).json({ error: "method not allowed (stateless MCP: POST only)" });
});

app.get("/health", (_req, res) => {
  res.json({ ok: true, deviceConnected: deviceHub.isConnected() });
});

// Brand: the SVG favicon (sharp at any size) + a tiny control-tower status face.
app.get(["/favicon.svg", "/icon.svg"], (_req, res) => {
  res.type("image/svg+xml").set("Cache-Control", "public, max-age=86400").send(ICON_SVG);
});
// Browsers probe /favicon.ico by default; hand them the vector instead.
app.get("/favicon.ico", (_req, res) => res.redirect(301, "/favicon.svg"));

app.get("/", (_req, res) => {
  const connected = deviceHub.isConnected();
  res.type("html").send(`<!doctype html><html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Abacad</title><link rel="icon" type="image/svg+xml" href="/icon.svg">
<style>
  :root{color-scheme:dark}
  *{margin:0;box-sizing:border-box}
  body{font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;
       background:#0d1420;color:#e7eef7;min-height:100vh;display:grid;place-items:center;padding:32px}
  .card{width:100%;max-width:420px}
  .top{display:flex;align-items:center;gap:14px;margin-bottom:22px}
  .top img{width:52px;height:52px;border-radius:13px;background:#fff}
  h1{font-size:20px;font-weight:700;letter-spacing:-.01em}
  .sub{font-size:13px;color:#93a4b8}
  .status{display:flex;align-items:center;gap:9px;padding:12px 14px;border-radius:12px;
          background:#111c2c;border:1px solid #1d2a3d;font-size:14px;margin-bottom:16px}
  .dot{width:10px;height:10px;border-radius:50%}
  .on{background:#3fcf7a}.off{background:#fd605e}
  ul{list-style:none;font-size:13px;color:#9fb0c4}
  li{padding:7px 0;border-top:1px solid #16202f;display:flex;justify-content:space-between;gap:12px}
  code{color:#cfe0f0;font-family:ui-monospace,SFMono-Regular,Menlo,monospace}
</style></head><body><div class="card">
  <div class="top"><img src="/icon.svg" alt="Abacad"><div>
    <h1>Abacad</h1><div class="sub">A device interface for agents.</div></div></div>
  <div class="status"><span class="dot ${connected ? "on" : "off"}"></span>
    ${connected ? "Device connected" : "Waiting for a device on /device"}</div>
  <ul>
    <li><span>Agent MCP endpoint</span><code>POST /mcp</code></li>
    <li><span>Device WebSocket</span><code>/device</code></li>
    <li><span>Health</span><code>GET /health</code></li>
  </ul>
</div></body></html>`);
});

const httpServer = createServer(app);

// The phone dials in here and holds the connection open.
const wss = new WebSocketServer({ server: httpServer, path: "/device" });
wss.on("connection", (ws) => deviceHub.attach(ws));

httpServer.listen(PORT, () => {
  console.error(`[abacad] agent MCP endpoint : http://localhost:${PORT}/mcp`);
  console.error(`[abacad] device WebSocket   : ws://<this-machine-LAN-IP>:${PORT}/device`);
  console.error(`[abacad] health            : http://localhost:${PORT}/health`);
  console.error(`[abacad] waiting for a device to connect on /device ...`);
});
