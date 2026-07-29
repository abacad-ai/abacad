import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import type { ActivityItem } from "@/lib/api";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// relativeTime renders a compact "how long ago" for a unix-millis (or ISO)
// timestamp: "just now", "12s ago", "3m ago", "2h ago", "4d ago".
export function relativeTime(input: number | string): string {
  const then = typeof input === "number" ? input : Date.parse(input);
  if (!Number.isFinite(then)) return "";
  const secs = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

// untilTime renders a compact "time remaining" for a future unix-millis (or ISO)
// timestamp: "expired", "in 45s", "in 12m", "in 3h", "in 2d". Sibling to
// relativeTime, which only ever looks backward.
export function untilTime(input: number | string): string {
  const then = typeof input === "number" ? input : Date.parse(input);
  if (!Number.isFinite(then)) return "";
  const secs = Math.round((then - Date.now()) / 1000);
  if (secs <= 0) return "expired";
  if (secs < 60) return `in ${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `in ${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `in ${hours}h`;
  return `in ${Math.floor(hours / 24)}d`;
}

// clockTime renders a unix-millis timestamp as HH:MM:SS for activity rows.
export function clockTime(ts: number): string {
  return new Date(ts).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

// actorText names who caused an activity row, for its meta line. Prefers the
// label snapshotted at write time (an API key's name, the signing-in address) and
// falls back to the id.
//
// Returns "" when the row has no actor — history recorded before the trail
// tracked provenance, or an event whose actor is genuinely unknown, like a failed
// sign-in. Callers omit the segment entirely in that case: printing "unknown"
// would read as a claim about the row rather than an absence of data.
export function actorText(a: ActivityItem): string {
  const name = a.actor_label || a.actor_id;
  if (!name) return "";
  switch (a.actor_kind) {
    case "apikey":
      return `key ${name}`;
    case "ssh":
      return `ssh ${name}`;
    case "device":
      return ""; // the row already names the device; repeating it is noise
    default:
      return name; // a session: the account email
  }
}

// locationText renders where an activity came from: "London, GB", or just "GB"
// when the database knew the country but not the city (common), or "" when the
// relay has no geo database or the address was private.
//
// Country leads because it is the trustworthy half. City-level geolocation is
// often the wrong city in the right region, and for mobile carriers, VPNs and
// CGNAT it can be off by a country — so it reads as a hint next to the country
// code, never as a standalone claim about where someone was.
export function locationText(a: ActivityItem): string {
  if (!a.country) return "";
  return a.city ? `${a.city}, ${a.country}` : a.country;
}
