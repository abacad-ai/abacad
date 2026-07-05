// The wire protocol between the server and the Android device, over the
// /device WebSocket. This tiny file is the seed of the future `contract/`.

export type Method = "ui_tree" | "screenshot" | "tap" | "swipe" | "wake" | "sleep";

/** Agent -> phone. `id` correlates the reply. */
export interface Command {
  id: string;
  method: Method;
  params?: Record<string, unknown>;
}

/** phone -> agent. */
export interface Reply {
  id: string;
  ok: boolean;
  result?: unknown;
  error?: string;
}

export interface UiTreeNode {
  cls: string;
  text: string;
  id: string;
  clickable: boolean;
  bounds: [number, number, number, number]; // [left, top, right, bottom]
}

export interface UiTreeResult {
  pkg: string;
  nodes: UiTreeNode[];
}

export interface ScreenshotResult {
  w: number;
  h: number;
  png_base64: string;
}

export interface TapResult {
  dispatched: boolean;
}

export interface WakeResult {
  screen_on: boolean;
  keyguard_secure: boolean;
  unlocked: boolean;
  note: string;
}

export interface SleepResult {
  locked: boolean;
}
