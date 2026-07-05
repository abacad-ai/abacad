// The wire protocol between the server and the Android device, over the
// /device WebSocket. This tiny file is the seed of the future `contract/`.
//
// The agent drives the device the way a human would: look at the screen,
// touch it, type, and press the nav keys. Power is the device's own affair —
// its normal display timeout sleeps it; auto-wake (in the app) brings it back
// transparently, so there are no wake/sleep methods here.

export type Method =
  | "screenshot"
  | "tap"
  | "long_press"
  | "swipe"
  | "input_text"
  | "back"
  | "home"
  | "recents";

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

/** The on-screen accessibility tree, delivered alongside a screenshot. */
export interface UiTree {
  pkg: string;
  nodes: UiTreeNode[];
}

export interface ScreenshotResult {
  w: number;
  h: number;
  png_base64: string;
  /** Present when the screenshot was requested with include_ui_tree. */
  tree?: UiTree;
}

/** tap / long_press / swipe all report whether the gesture was dispatched. */
export interface GestureResult {
  dispatched: boolean;
}

/** input_text reports whether the text was set on the focused field. */
export interface InputTextResult {
  set: boolean;
}

/** back / home / recents report whether the global action was performed. */
export interface GlobalActionResult {
  performed: boolean;
}
