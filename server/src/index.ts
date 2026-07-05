import express from "express";
import { createServer } from "node:http";
import { WebSocketServer } from "ws";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { buildMcpServer } from "./mcp.js";
import { deviceHub } from "./device.js";

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
