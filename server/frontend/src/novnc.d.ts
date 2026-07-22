// noVNC ships its own types for the package root but not always for the deep
// ESM path we import, so declare the RFB module we use. RFB's surface is small
// here (constructor + a few properties/events), typed loosely on purpose.
declare module "@novnc/novnc" {
  export default class RFB extends EventTarget {
    constructor(target: HTMLElement, url: string, options?: Record<string, unknown>);
    scaleViewport: boolean;
    background: string;
    viewOnly: boolean;
    disconnect(): void;
  }
}
