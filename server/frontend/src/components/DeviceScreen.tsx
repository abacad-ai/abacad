import { useEffect, useState } from "react";
import { LoaderCircle, Monitor, Smartphone } from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { cn } from "@/lib/utils";
import { type FormFactor } from "@/lib/devices";

export const SCREENSHOT_GAP_MS = 2000;

// The device screen — an absolutely-positioned layer inside the frame. The
// server keeps each device's last screenshot, so we can render one instantly:
//
//   - offline, with a stored frame: show it grayscaled (the last thing it saw),
//   - offline, no stored frame:     the "signal lost" placeholder,
//   - online:                       show the stored frame at once, then live-poll
//                                   fresh frames (each also becomes the new stored
//                                   one server-side) and swap them in.
//
// The frame is sized to the screenshot's own aspect ratio, so object-contain
// fills it exactly — the capture is shown whole, never cropped or stretched. On
// each image load we report its natural aspect ratio up to the frame via onAspect.
export function DeviceScreen({
  device,
  factor,
  onAspect,
}: {
  device: DeviceView;
  factor: FormFactor;
  onAspect: (ratio: number | null) => void;
}) {
  const [liveFrame, setLiveFrame] = useState<string | null>(null);
  const [broken, setBroken] = useState(false);

  const base = api.deviceScreenshotUrl(device.id);
  // The stored last screenshot, keyed by its capture time so the browser refetches
  // only when it actually changes. Absent until the device has ever been captured.
  const savedFrame = device.screenshot_at ? `${base}?v=${device.screenshot_at}` : null;

  // Start clean when switching to a different device.
  useEffect(() => {
    setLiveFrame(null);
    setBroken(false);
  }, [device.id]);

  // Live poll while online. Each fetch captures a fresh frame (the server stores
  // it as the device's new last screenshot) which we preload, then swap in — so
  // the visible image never flashes to empty mid-load. The last live frame is
  // kept when the device drops, so it lingers (grayscaled) instead of vanishing.
  useEffect(() => {
    if (!device.online) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    let seq = 0;

    const loadNext = () => {
      const url = `${base}?live=1&t=${Date.now()}_${seq++}`;
      const img = new Image();
      img.onload = () => {
        if (!alive) return;
        setLiveFrame(url);
        setBroken(false);
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.onerror = () => {
        if (!alive) return;
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.src = url;
    };

    loadNext();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [device.online, device.id, base]);

  const OfflineIcon = factor === "handset" ? Smartphone : Monitor;
  const shown = !broken ? liveFrame ?? savedFrame : null;

  if (shown) {
    return (
      <img
        src={shown}
        alt={`${device.name} screen`}
        onLoad={(e) => {
          const img = e.currentTarget;
          if (img.naturalWidth && img.naturalHeight) onAspect(img.naturalWidth / img.naturalHeight);
        }}
        onError={() => setBroken(true)}
        className={cn(
          "absolute inset-0 h-full w-full object-contain",
          !device.online && "grayscale",
        )}
      />
    );
  }

  // No frame to show: waiting for the first capture while online, or a device
  // that has never been captured and is now offline.
  if (device.online) {
    return (
      <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
        <LoaderCircle size={20} className="animate-spin" />
        <span className="font-mono text-[10px] uppercase tracking-wider">Capturing</span>
      </div>
    );
  }
  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 text-ink-subtle">
      <OfflineIcon size={factor === "handset" ? 24 : 30} strokeWidth={1.25} />
      <span className="font-mono text-[10px] uppercase tracking-[0.22em]">Signal lost</span>
    </div>
  );
}

// A lightweight frame: a hairline border and a soft shadow. When a screenshot
// has loaded, the frame takes that image's exact aspect ratio, so the capture is
// shown at its true shape — never cropped, never stretched. Until then (loading
// or offline) it falls back to a device-shaped ratio: tall for a phone, wide for
// a screen. Corner radius still signals form factor — very rounded for a phone,
// gently rounded for a screen.
export function DeviceFrame({
  factor,
  aspect,
  maxWidth,
  children,
}: {
  factor: FormFactor;
  aspect: number | null;
  maxWidth?: string; // Tailwind max-w-* override; defaults to the grid-card cap.
  children: React.ReactNode;
}) {
  const radius = factor === "handset" ? "rounded-[1.7rem]" : "rounded-[12px]";
  const ratio = aspect ?? (factor === "handset" ? 9 / 18.5 : 16 / 10);
  const cap = maxWidth ?? (factor === "handset" ? "max-w-[176px]" : "");
  return (
    <div className={`mx-auto w-full ${cap}`}>
      <div
        className={`relative overflow-hidden border border-border bg-surface-raised shadow-[0_10px_24px_-16px_var(--shadow-strong)] transition-transform duration-200 hover:-translate-y-0.5 ${radius}`}
        style={{ aspectRatio: ratio }}
      >
        {children}
      </div>
    </div>
  );
}
