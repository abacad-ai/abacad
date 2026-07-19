// Theme preference: "auto" (follow the system — the default) or a forced
// scheme. The palette itself lives in tokens.css; forcing works by stamping
// data-theme on <html>, which the generated CSS keys off. index.html stamps
// the stored value inline before first paint; this module owns it afterwards.

export type ThemePreference = "auto" | "light" | "dark";

const STORAGE_KEY = "abacad-theme";

export function getThemePreference(): ThemePreference {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    return v === "light" || v === "dark" ? v : "auto";
  } catch {
    return "auto";
  }
}

export function setThemePreference(pref: ThemePreference): void {
  try {
    if (pref === "auto") localStorage.removeItem(STORAGE_KEY);
    else localStorage.setItem(STORAGE_KEY, pref);
  } catch {
    // Private browsing: the choice still applies for this page view.
  }
  applyThemePreference();
}

/** Sync <html data-theme> and the browser-chrome color with the stored preference. */
export function applyThemePreference(): void {
  const pref = getThemePreference();
  if (pref === "auto") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = pref;

  // theme-color can't react to data-theme via media queries, so mirror the
  // effective canvas color onto both meta tags.
  const canvas = getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim();
  if (canvas) {
    document
      .querySelectorAll('meta[name="theme-color"]')
      .forEach((m) => m.setAttribute("content", canvas));
  }
}

// In auto mode the effective scheme follows the OS — re-sync when it flips.
window
  .matchMedia("(prefers-color-scheme: light)")
  .addEventListener("change", () => applyThemePreference());
