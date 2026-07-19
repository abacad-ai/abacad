import { useEffect, useState } from "react";
import { ImageOff, LoaderCircle, Monitor, Smartphone } from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { relativeTime } from "@/lib/utils";
import { type FormFactor } from "@/lib/devices";

export const SCREENSHOT_GAP_MS = 2000;

// The live screen — an absolutely-positioned layer inside the frame. The frame
// is sized to the screenshot's own aspect ratio, so object-contain fills it
// exactly: the capture is shown whole, never cropped or stretched. On each load
// we report the image's natural aspect ratio up to the frame via onAspect.
export function DeviceScreen({
  device,
  factor,
  onAspect,
}: {
  device: DeviceView;
  factor: FormFactor;
  onAspect: (ratio: number | null) => void;
}) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!device.online) {
      setSrc(null);
      setFailed(false);
      onAspect(null);
      return;
    }

    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    let seq = 0;

    const loadNext = () => {
      const url = `${api.deviceScreenshotUrl(device.id)}?t=${Date.now()}_${seq++}`;
      const img = new Image();
      img.onload = () => {
        if (!alive) return;
        setSrc(url);
        setFailed(false);
        if (img.naturalWidth && img.naturalHeight) {
          onAspect(img.naturalWidth / img.naturalHeight);
        }
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.onerror = () => {
        if (!alive) return;
        setFailed(true);
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.src = url;
    };

    loadNext();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [device.online, device.id, onAspect]);

  const OfflineIcon = factor === "handset" ? Smartphone : Monitor;

  return (
    <>
      {device.online ? (
        <>
          {src && (
            <img
              src={src}
              alt={`${device.name} screen`}
              className="absolute inset-0 h-full w-full object-contain"
            />
          )}
          {!src && !failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
              <LoaderCircle size={20} className="animate-spin" />
              <span className="font-mono text-[10px] uppercase tracking-wider">Capturing</span>
            </div>
          )}
          {!src && failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-3 text-center text-ink-subtle">
              <ImageOff size={22} />
              <span className="font-mono text-[10px] uppercase leading-4 tracking-wider">No capture</span>
            </div>
          )}
        </>
      ) : (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 text-ink-subtle">
          <OfflineIcon size={factor === "handset" ? 24 : 30} strokeWidth={1.25} />
          <span className="font-mono text-[10px] uppercase tracking-[0.22em]">Signal lost</span>
          {device.last_seen && (
            <span className="font-mono text-[10px]">seen {relativeTime(device.last_seen)}</span>
          )}
        </div>
      )}
    </>
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
