import { useState } from "react";
import { Monitor, Moon, Palette, Sun } from "lucide-react";
import { Card } from "@/components/ui/card";
import { SshKeysCard } from "@/components/SshKeysCard";
import { PageHeader } from "@/components/PageHeader";
import { cn } from "@/lib/utils";
import { getThemePreference, setThemePreference, type ThemePreference } from "@/lib/theme";

// Settings holds account credentials that aren't API keys (SSH access keys live
// here) plus console preferences. API keys have their own Access page.
export function SettingsPage() {
  return (
    <div>
      <PageHeader title="Settings" />
      <SshKeysCard />
      <AppearanceCard />
    </div>
  );
}

const THEME_OPTIONS: { value: ThemePreference; label: string; icon: typeof Monitor }[] = [
  { value: "auto", label: "Auto", icon: Monitor },
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
];

// Appearance picker — a segmented Auto / Light / Dark control wired to the shared
// theme helpers. "Auto" follows the system; a forced choice stamps data-theme on
// <html>. This is the single home for the theme setting (moved out of the header).
function AppearanceCard() {
  const [pref, setPref] = useState<ThemePreference>(getThemePreference);

  const choose = (next: ThemePreference) => {
    setThemePreference(next);
    setPref(next);
  };

  return (
    <Card className="mt-6 overflow-hidden">
      <div className="flex items-start gap-3 border-b border-border p-5 sm:p-6">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
          <Palette size={19} />
        </span>
        <div>
          <h2 className="font-display text-lg font-bold text-ink">Appearance</h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-ink-muted">
            Choose the console's color scheme. Auto follows your system setting.
          </p>
        </div>
      </div>

      <div className="p-5 sm:p-6">
        <div
          role="radiogroup"
          aria-label="Color scheme"
          className="grid max-w-md grid-cols-3 gap-1 rounded-md border border-border bg-canvas p-1"
        >
          {THEME_OPTIONS.map(({ value, label, icon: Icon }) => {
            const active = pref === value;
            return (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={active}
                onClick={() => choose(value)}
                className={cn(
                  "flex h-10 items-center justify-center gap-2 rounded-[6px] text-[13px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                  active ? "bg-surface-raised text-ink shadow-[0_1px_2px_var(--shadow)]" : "text-ink-muted hover:text-ink",
                )}
              >
                <Icon size={16} />
                {label}
              </button>
            );
          })}
        </div>
      </div>
    </Card>
  );
}
