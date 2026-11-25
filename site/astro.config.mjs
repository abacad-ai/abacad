// @ts-check
import sitemap from "@astrojs/sitemap";
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import starlightLlmsTxt from "starlight-llms-txt";

import { sidebar } from "./src/sidebar.mjs";

// The marketing page lives at `/` and the docs at `/docs/**`. The docs content
// is nested under `src/content/docs/docs/`, which is why this project has no
// Astro `base`: the landing page and Starlight both sit at the site root.
export default defineConfig({
  site: "https://abacad.ai",
  trailingSlash: "always",
  integrations: [
    starlight({
      title: "abacad docs",
      description:
        "Set up abacad, a self-hosted VNC hub: connect the VNC servers your machines already run, open them in a browser, and hand the same stream to an agent. Platform setup, MCP, security, and troubleshooting.",
      logo: {
        src: "./src/assets/mark.svg",
        alt: "abacad",
      },
      favicon: "/favicon.svg",
      customCss: ["./src/styles/tokens.css", "./src/styles/docs.css"],
      components: {
        Head: "./src/components/Head.astro",
      },
      defaultLocale: "root",
      locales: {
        root: { label: "English", lang: "en" },
        zh: { label: "简体中文", lang: "zh-CN" },
      },
      sidebar,
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/abacad-ai/abacad",
        },
      ],
      editLink: {
        baseUrl:
          "https://github.com/abacad-ai/abacad/edit/main/site/",
      },
      lastUpdated: false,
      // We ship our own branded 404 at src/pages/404.astro.
      disable404Route: true,
      // The docs are a curated public surface, not a mirror of the repo's
      // internal notes. Keep the edit affordance because the source is public.
      pagination: true,
      plugins: [
        starlightLlmsTxt({
          projectName: "abacad",
          description:
            "A self-hosted VNC hub. Connect the VNC servers your machines already run, open any of them in a browser, and hand the same stream to an agent over MCP or a ready-made skill. No software on the devices, no cloud, no relay.",
        }),
      ],
    }),
    sitemap(),
  ],
});
