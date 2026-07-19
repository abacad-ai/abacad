import { Monitor, Moon, Sun } from "lucide-react";
import { useState } from "react";
import { getThemePreference, setThemePreference, type ThemePreference } from "@/lib/theme";

const NEXT: Record<ThemePreference, ThemePreference> = { auto: "light", light: "dark", dark: "auto" };
const LABEL: Record<ThemePreference, string> = {
  auto: "Theme: auto (follows system)",
  light: "Theme: light",
  dark: "Theme: dark",
};
const ICON: Record<ThemePreference, typeof Monitor> = { auto: Monitor, light: Sun, dark: Moon };

/** Cycles auto → light → dark. Auto is the default; the icon shows the current choice. */
export function ThemeToggle() {
  const [pref, setPref] = useState<ThemePreference>(getThemePreference);

  const cycle = () => {
    const next = NEXT[pref];
    setThemePreference(next);
    setPref(next);
  };

  const Icon = ICON[pref];
  const label = `${LABEL[pref]} — switch to ${NEXT[pref]}`;
  return (
    <button
      type="button"
      onClick={cycle}
      className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      title={label}
      aria-label={label}
    >
      <Icon size={17} />
    </button>
  );
}
