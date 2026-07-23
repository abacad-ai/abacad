// @ts-check
import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";
import sitemap from "@astrojs/sitemap";

// The docs site is served as a static subtree under abacad.ai/docs by the Go
// server (//go:embed all:docs-dist + http.StripPrefix("/docs/", …)). `base` must
// match that mount point so every asset/link URL resolves under /docs, and `site`
// is the real origin so canonical/OG/sitemap URLs are absolute and correct.
export default defineConfig({
  site: "https://abacad.ai",
  base: "/docs",
  trailingSlash: "always",
  integrations: [
    starlight({
      title: "abacad docs",
      description:
        "How abacad connects a phone, laptop, or browser to a coding agent — the tool surface, SSH access, and running a device hands-off. Honest about what ships today.",
      logo: {
        src: "./src/assets/mark.svg",
        alt: "abacad",
      },
      favicon: "/favicon.svg",
      customCss: ["./src/styles/custom.css"],
      // A single honest status convention runs through every reference table.
      sidebar: [
        { label: "What abacad is", link: "/" },
        {
          label: "Reference",
          items: [
            { label: "Tool reference", slug: "reference/tools" },
            { label: "Screen recording", slug: "reference/screen-recording" },
            { label: "Transport", slug: "reference/transport" },
            { label: "Reading status markers", slug: "reference/status-markers" },
          ],
        },
        {
          label: "Guides",
          items: [
            { label: "SSH access", slug: "guides/ssh" },
            { label: "Running a phone hands-off", slug: "guides/running-hands-off" },
          ],
        },
        {
          label: "Security",
          items: [{ label: "Security & trust", slug: "security" }],
        },
      ],
      // Show the "edit"/last-updated affordances off; this is a curated public
      // mirror, not the internal source of truth.
      editLink: undefined,
      lastUpdated: false,
    }),
    sitemap(),
  ],
});
