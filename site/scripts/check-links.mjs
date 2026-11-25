// Fails the build on a broken internal link or asset. External URLs are not
// fetched: CI should not depend on the internet, and link rot belongs in a
// separate audit.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";

const DIST = "dist";

function walk(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = join(dir, entry.name);
    return entry.isDirectory() ? walk(full) : [full];
  });
}

function isFile(path) {
  try {
    return statSync(path).isFile();
  } catch {
    return false;
  }
}

const htmlFiles = walk(DIST).filter((file) => file.endsWith(".html"));
const missing = [];
const missingAnchors = [];

for (const file of htmlFiles) {
  const html = readFileSync(file, "utf8");
  const pageDir = `/${relative(DIST, file)
    .split(sep)
    .join("/")
    .replace(/index\.html$/, "")
    .replace(/\.html$/, "")}`.replace(/\/+/g, "/");

  for (const match of html.matchAll(/\b(?:href|src)="([^"]+)"/g)) {
    let url = match[1];
    if (
      !url ||
      url.startsWith("#") ||
      url.startsWith("http:") ||
      url.startsWith("https:") ||
      url.startsWith("//") ||
      url.startsWith("mailto:") ||
      url.startsWith("tel:") ||
      url.startsWith("data:") ||
      url.startsWith("javascript:")
    ) {
      continue;
    }

    const [pathPart, hash] = match[1].split("#");
    url = pathPart.split("?")[0];
    if (!url) {
      if (hash && hash !== "_top") {
        const page = file;
        const hasAnchor = new RegExp(`(?:id|name)="${hash.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}"`).test(
          readFileSync(page, "utf8"),
        );
        if (!hasAnchor) missingAnchors.push({ page: relative(DIST, file), link: match[1] });
      }
      continue;
    }

    let target;
    if (url.startsWith("/")) {
      target = url;
    } else {
      target = `${pageDir}/${url}`;
    }

    const path = target.split("/").filter(Boolean).join("/");
    const candidates = target.endsWith("/")
      ? [join(DIST, path, "index.html")]
      : [join(DIST, path), join(DIST, `${path}.html`), join(DIST, path, "index.html")];

    const targetFile = candidates.find(isFile);
    if (!targetFile) {
      missing.push({ page: relative(DIST, file), link: match[1], resolved: target });
      continue;
    }
    if (hash && hash !== "_top") {
      const escaped = hash.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const hasAnchor = new RegExp(`(?:id|name)="${escaped}"`).test(readFileSync(targetFile, "utf8"));
      if (!hasAnchor) {
        missingAnchors.push({ page: relative(DIST, file), link: match[1], resolved: `${target}#${hash}` });
      }
    }
  }
}

if (missing.length || missingAnchors.length) {
  if (missing.length) {
    console.error(`broken internal links: ${missing.length}`);
    for (const item of missing.slice(0, 50)) {
      console.error(`  ${item.page}: ${item.link}  →  ${item.resolved}`);
    }
  }
  if (missingAnchors.length) {
    console.error(`missing anchors: ${missingAnchors.length}`);
    for (const item of missingAnchors.slice(0, 50)) {
      console.error(`  ${item.page}: ${item.link}  →  ${item.resolved}`);
    }
  }
  process.exit(1);
}

console.log(`internal links and anchors ok (${htmlFiles.length} HTML pages)`);
