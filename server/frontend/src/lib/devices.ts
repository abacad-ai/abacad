// Platform classification for devices — shared by the grid (which groups by
// platform) and the detail page (which labels a single device).
//
// Devices carry a platform string (e.g. "android", "macos"). It can be blank on
// older devices, so we fall back to inferring from the name. Everything maps to
// a display label and a form factor, which drives both the section a device
// lands in and the frame it wears.

import { type DeviceView } from "@/lib/api";

export type FormFactor = "handset" | "desktop";

// Liveness rank for sorting: active (2) > asleep (1) > offline (0). An asleep
// device is still online, just idle, so it outranks an offline one.
function liveness(d: DeviceView): number {
  if (!d.online) return 0;
  return d.activity === "asleep" ? 1 : 2;
}

export interface PlatformInfo {
  label: string;
  factor: FormFactor;
}

export interface PlatformGroup extends PlatformInfo {
  key: string;
  devices: DeviceView[];
}

const KNOWN_PLATFORMS: Record<string, PlatformInfo> = {
  macos: { label: "macOS", factor: "desktop" },
  mac: { label: "macOS", factor: "desktop" },
  darwin: { label: "macOS", factor: "desktop" },
  osx: { label: "macOS", factor: "desktop" },
  windows: { label: "Windows", factor: "desktop" },
  win32: { label: "Windows", factor: "desktop" },
  linux: { label: "Linux", factor: "desktop" },
  android: { label: "Android", factor: "handset" },
  ios: { label: "iOS", factor: "handset" },
  ipados: { label: "iPadOS", factor: "handset" },
  browser: { label: "Browser", factor: "desktop" },
};

// The platforms you can create a device for, in picker order. Each one has a
// client that can connect; add "windows"/"linux" here once their apps ship —
// KNOWN_PLATFORMS already knows their label and frame.
export const NEW_DEVICE_PLATFORMS = ["android", "macos", "browser"];

export function platformInfo(platform: string): PlatformInfo {
  return KNOWN_PLATFORMS[platform] ?? { label: platform, factor: "desktop" };
}

// Public client builds, keyed by platform label. The server publishes release
// artifacts at /downloads/<file> (a plain directory on the deploy volume), so a
// new platform's client is a naming convention plus a file copy. Platforms with
// no shipped client are simply absent — the UI then offers no download.
const CLIENT_DOWNLOADS: Record<string, string> = {
  macOS: "/downloads/abacad-macos-latest.dmg",
  Android: "/downloads/abacad-android-latest.apk",
};

export function clientDownload(info: PlatformInfo): string | null {
  return CLIENT_DOWNLOADS[info.label] ?? null;
}

// Section order — desktops first, then handsets, with unrecognized labels last.
const GROUP_ORDER = ["macOS", "Windows", "Linux", "Desktop", "Browser", "iPadOS", "iOS", "Android", "Mobile", "Other"];

function classifyText(text: string): PlatformInfo | null {
  const t = text.toLowerCase();
  if (/macbook|imac|mac ?mini|mac ?studio|\bmac\b|macos|osx|darwin/.test(t)) return { label: "macOS", factor: "desktop" };
  if (/windows|\bwin\b|\bpc\b|thinkpad|surface/.test(t)) return { label: "Windows", factor: "desktop" };
  if (/linux|ubuntu|debian|fedora|arch/.test(t)) return { label: "Linux", factor: "desktop" };
  if (/iphone|ipad|\bios\b/.test(t)) return { label: "iOS", factor: "handset" };
  if (/android|pixel|galaxy|samsung|\bzte\b|xiaomi|redmi|oneplus|oppo|vivo|nexus|moto|huawei|honor|nokia/.test(t))
    return { label: "Android", factor: "handset" };
  if (/phone|mobile|tablet/.test(t)) return { label: "Mobile", factor: "handset" };
  if (/desktop|laptop|computer/.test(t)) return { label: "Desktop", factor: "desktop" };
  if (/browser|chrome|safari|firefox|\bedge\b|\btab\b|kiosk/.test(t)) return { label: "Browser", factor: "desktop" };
  return null;
}

export function resolvePlatform(device: DeviceView): PlatformInfo {
  const p = (device.platform ?? "").trim().toLowerCase();
  return (
    (p ? KNOWN_PLATFORMS[p] ?? classifyText(p) : null) ??
    classifyText(device.name) ?? { label: "Other", factor: "desktop" }
  );
}

export function groupDevices(devices: DeviceView[]): PlatformGroup[] {
  const groups = new Map<string, PlatformGroup>();
  for (const device of devices) {
    const info = resolvePlatform(device);
    const key = info.label.toLowerCase();
    let group = groups.get(key);
    if (!group) {
      group = { key, label: info.label, factor: info.factor, devices: [] };
      groups.set(key, group);
    }
    group.devices.push(device);
  }

  const rank = (label: string) => {
    const index = GROUP_ORDER.indexOf(label);
    return index === -1 ? GROUP_ORDER.length : index;
  };

  return [...groups.values()]
    .map((group) => ({
      ...group,
      // Sort within a group: active first, then asleep (still online), then offline.
      devices: [...group.devices].sort(
        (a, b) => liveness(b) - liveness(a) || a.name.localeCompare(b.name),
      ),
    }))
    .sort((a, b) => rank(a.label) - rank(b.label) || a.label.localeCompare(b.label));
}
