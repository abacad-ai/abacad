import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

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
