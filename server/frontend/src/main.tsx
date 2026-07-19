import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "@/App";
import { applyThemePreference } from "@/lib/theme";
import "./index.css";

// index.html stamps data-theme pre-paint; this re-syncs it and the
// browser-chrome color once the stylesheet (and --canvas) is loaded.
applyThemePreference();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
