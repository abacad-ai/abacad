import { WebSocket } from "ws";
import type { Command, Method, Reply } from "./protocol.js";

interface Pending {
  resolve: (result: unknown) => void;
  reject: (err: Error) => void;
  timer: NodeJS.Timeout;
}

/**
 * Owns the single connected Android device (v0 is one device). Turns each
 * `send(method, params)` into a Promise, correlated to the device's reply by id.
 * This is the whole "relay" for now: agent tool call -> here -> device -> here.
 */
class DeviceHub {
  private ws: WebSocket | null = null;
  private pending = new Map<string, Pending>();
  private seq = 0;

  attach(ws: WebSocket): void {
    if (this.ws) {
      try {
        this.ws.close();
      } catch {
        /* ignore */
      }
    }
    this.ws = ws;
    ws.on("message", (data) => this.onMessage(data.toString()));
    ws.on("close", () => {
      if (this.ws === ws) this.ws = null;
      this.failAll(new Error("device disconnected"));
      console.error("[device] disconnected");
    });
    ws.on("error", (e) => console.error("[device] socket error:", (e as Error).message));
    console.error("[device] connected");
  }

  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  send(method: Method, params?: Record<string, unknown>, timeoutMs = 15000): Promise<unknown> {
    const ws = this.ws;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error("no device connected — open the Abacad app and connect it to this server"));
    }
    const id = String(++this.seq);
    const cmd: Command = { id, method, params };
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`device timed out on ${method} (${timeoutMs}ms)`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      ws.send(JSON.stringify(cmd), (err) => {
        if (err) {
          clearTimeout(timer);
          this.pending.delete(id);
          reject(err);
        }
      });
    });
  }

  private onMessage(raw: string): void {
    let reply: Reply;
    try {
      reply = JSON.parse(raw) as Reply;
    } catch {
      console.error("[device] non-JSON message:", raw.slice(0, 200));
      return;
    }
    const p = this.pending.get(reply.id);
    if (!p) return;
    clearTimeout(p.timer);
    this.pending.delete(reply.id);
    if (reply.ok) p.resolve(reply.result);
    else p.reject(new Error(reply.error ?? "device reported an error"));
  }

  private failAll(err: Error): void {
    for (const p of this.pending.values()) {
      clearTimeout(p.timer);
      p.reject(err);
    }
    this.pending.clear();
  }
}

export const deviceHub = new DeviceHub();
