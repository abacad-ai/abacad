// noVNC ships no TypeScript declarations; this mirrors the parts of
// docs/API.md (node_modules/@novnc/novnc/docs/API.md) that the viewer uses.
// Extend it when a new entry point is needed, and re-check it on every bump.
declare module "@novnc/novnc" {
  export interface RFBCredentials {
    username?: string
    password?: string
    target?: string
  }

  export interface RFBOptions {
    shared?: boolean
    credentials?: RFBCredentials
    repeaterID?: string
    wsProtocols?: string[]
  }

  export interface RFBCapabilities {
    power?: boolean
  }

  export interface RFBEventMap {
    bell: Event
    capabilities: CustomEvent<{ capabilities: RFBCapabilities }>
    clipboard: CustomEvent<{ text: string }>
    clippingviewport: CustomEvent<boolean>
    connect: Event
    credentialsrequired: CustomEvent<{ types: string[] }>
    desktopname: CustomEvent<{ name: string }>
    disconnect: CustomEvent<{ clean: boolean }>
    securityfailure: CustomEvent<{ status: number; reason?: string }>
    serververification: CustomEvent<{ type: string; publickey?: Uint8Array }>
  }

  export default class RFB extends EventTarget {
    constructor(
      target: HTMLElement,
      urlOrChannel: string | WebSocket | RTCDataChannel,
      options?: RFBOptions,
    )

    background: string
    readonly capabilities: RFBCapabilities
    readonly clippingViewport: boolean
    clipViewport: boolean
    compressionLevel: number
    dragViewport: boolean
    focusOnClick: boolean
    qualityLevel: number
    resizeSession: boolean
    scaleViewport: boolean
    showDotCursor: boolean
    viewOnly: boolean

    approveServer(): void
    blur(): void
    clipboardPasteFrom(text: string): void
    disconnect(): void
    focus(options?: FocusOptions): void
    getImageData(): ImageData
    machineReboot(): void
    machineReset(): void
    machineShutdown(): void
    sendCredentials(credentials: RFBCredentials): void
    sendCtrlAltDel(): void
    sendKey(keysym: number, code: string | null, down?: boolean): void
    toBlob(callback: (blob: Blob) => void, type?: string, quality?: number): void
    toDataURL(type?: string, encoderOptions?: number): string

    addEventListener<K extends keyof RFBEventMap>(
      type: K,
      listener: (event: RFBEventMap[K]) => void,
      options?: boolean | AddEventListenerOptions,
    ): void
    addEventListener(
      type: string,
      listener: EventListenerOrEventListenerObject,
      options?: boolean | AddEventListenerOptions,
    ): void

    removeEventListener<K extends keyof RFBEventMap>(
      type: K,
      listener: (event: RFBEventMap[K]) => void,
      options?: boolean | EventListenerOptions,
    ): void
    removeEventListener(
      type: string,
      listener: EventListenerOrEventListenerObject,
      options?: boolean | EventListenerOptions,
    ): void
  }
}
