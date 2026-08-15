import { Link } from "react-router-dom";
import { Download, Globe, Laptop, LoaderCircle, Monitor, Smartphone, Terminal } from "lucide-react";
import { type Build } from "@/lib/api";
import { useManifest } from "@/lib/useManifest";
import { PublicLayout } from "@/components/PublicLayout";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { platformInfo } from "@/lib/devices";
import { cn } from "@/lib/utils";

// Public client downloads at /downloads — deliberately reachable with no account,
// since you install the client on a device before (or while) signing up. The
// buttons come from /downloads/manifest.json, so the page offers exactly the
// builds staged on the server: no dead links, and a newly published platform (or
// arch) shows up without a frontend change.
//
// Note the backend also owns /downloads/<file> (the artifacts + the manifest
// itself); this SPA route is the bare /downloads path, which Go's mux leaves to
// the SPA.

interface PlatformCard {
  key: string; // matches Build.platform
  label: string;
  icon: typeof Laptop;
  requirement: string;
  note: string;
  // Shown under the CLI download, for a one-line install path. A function, not a
  // string, so it reads the current origin at render — a self-hosted instance must
  // print its own host, not abacad.ai.
  cliHint?: () => string;
}

// The clients we describe, in display order. A platform with no published build
// is still listed (as "coming soon") — knowing it is planned beats a blank page.
const CATALOG: PlatformCard[] = [
  {
    key: "macos",
    label: "macOS",
    icon: Laptop,
    requirement: "macOS 14 Sonoma or later",
    note: "Grant permissions once, then use the restart-readiness checklist for secure or unattended operation.",
  },
  {
    key: "android",
    label: "Android",
    icon: Smartphone,
    requirement: "Android 11 or later",
    note: "One accessibility permission covers both vision and control — it survives reboots, so setup is once.",
  },
  {
    key: "windows",
    label: "Windows",
    icon: Monitor,
    requirement: "Windows 10 1809 or later",
    note: "The installer sets up the tray app for your account only — no administrator prompt — and can start it when you sign in. On a server with no desktop, take the CLI instead.",
  },
  {
    key: "linux",
    label: "Linux",
    icon: Terminal,
    requirement: "X11 · GTK4 for the app",
    note: "The .deb installs the desktop app and a systemd user service. On a headless box, take the CLI instead.",
    cliHint: () => `curl -fsSL ${window.location.origin}/install.sh | sh`,
  },
];

export function DownloadsPage() {
  const { builds, error: failed } = useManifest();

  const buildsFor = (key: string) => (builds ?? []).filter((b) => b.platform === key);
  // A platform the catalog doesn't describe still gets a card, so staging a file
  // on the server is always enough to offer it.
  const extraPlatforms = [...new Set((builds ?? []).map((b) => b.platform))].filter(
    (p) => !CATALOG.some((c) => c.key === p),
  );

  return (
    <PublicLayout>
      <div className="relative z-10 mx-auto w-full max-w-5xl flex-1 px-4 py-14 sm:px-6 sm:py-20">
        <div className="text-center">
          <p className="font-mono text-[11px] font-medium uppercase tracking-[0.28em] text-brand">downloads</p>
          <h1 className="mt-5 font-display text-4xl font-bold leading-[1.1] tracking-tight text-ink sm:text-5xl">
            Get the abacad client
          </h1>
          <p className="mx-auto mt-5 max-w-2xl text-base leading-7 text-ink-muted">
            Install it on the device you want your agent to reach. No account needed to download — you sign in only
            when you connect the device to your workspace.
          </p>
        </div>

        {failed && (
          <p className="mt-10 rounded-md border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger">
            Couldn't load the current builds ({failed}). Try again in a moment.
          </p>
        )}

        <div className="mt-12 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {CATALOG.map((card) => (
            <ClientCard key={card.key} card={card} builds={buildsFor(card.key)} loading={builds === null && !failed} />
          ))}
          {extraPlatforms.map((p) => (
            <ClientCard
              key={p}
              card={{
                key: p,
                label: platformInfo(p).label,
                icon: Laptop,
                requirement: "Latest build",
                note: "",
              }}
              builds={buildsFor(p)}
              loading={false}
            />
          ))}
          <BrowserCard />
        </div>

        <section className="mt-14">
          <h2 className="font-display text-xl font-bold text-ink">After you install</h2>
          <ol className="mt-5 grid gap-4 sm:grid-cols-3">
            <Step
              n={1}
              title="Add the device"
              body="Sign in to the console and add a device for the platform you just installed."
            />
            <Step
              n={2}
              title="Connect the client"
              body="Scan the device's QR code, or paste its connection URL into the client. That pairs it to your workspace."
            />
            <Step
              n={3}
              title="Point your agent at it"
              body="Register abacad's MCP endpoint with your agent and target the device by its id."
            />
          </ol>
          <Link to="/login" className={cn(buttonVariants({ variant: "outline" }), "mt-6")}>
            Open the console
          </Link>
        </section>
      </div>
    </PublicLayout>
  );
}

function ClientCard({ card, builds, loading }: { card: PlatformCard; builds: Build[]; loading: boolean }) {
  const Icon = card.icon;
  // Most platforms publish two things for the same machine: the app (a window,
  // a tray/menu-bar icon, a Pause button) and the CLI (the same agent, driven
  // from a terminal). They're separate downloads, so they get separate buttons —
  // and the group headings only appear when there is in fact a choice to make.
  const apps = byArch(builds.filter((b) => kindOf(b) === "app"));
  const clis = byArch(builds.filter((b) => kindOf(b) === "cli"));
  const both = apps.length > 0 && clis.length > 0;
  return (
    <Card className="flex flex-col p-5">
      <span className="flex h-9 w-9 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
        <Icon size={17} />
      </span>
      <h2 className="mt-3.5 font-display text-[15px] font-bold text-ink">{card.label}</h2>
      <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.14em] text-ink-subtle">{card.requirement}</p>
      {card.note && <p className="mt-2.5 text-sm leading-6 text-ink-muted">{card.note}</p>}

      <div className="mt-5 flex-1" />

      {loading ? (
        <span className="inline-flex h-11 items-center gap-2 text-sm text-ink-subtle">
          <LoaderCircle size={16} className="animate-spin" />
          Checking for a build
        </span>
      ) : apps.length + clis.length > 0 ? (
        <div className="flex flex-col gap-4">
          <DownloadGroup label={both ? "App" : null} builds={apps} />
          <DownloadGroup
            label={both ? "Command line" : null}
            builds={clis}
            variant="outline"
            hint={clis.length > 0 ? card.cliHint?.() : undefined}
          />
        </div>
      ) : (
        // No published artifact: say so plainly rather than offering a dead button.
        <span className="inline-flex h-11 items-center rounded-md border border-dashed border-border px-3 text-sm text-ink-subtle">
          Not available yet
        </span>
      )}
    </Card>
  );
}

// One arch per button, arch-labeled only when a platform ships more than one
// (Linux CLI is amd64 + arm64). Versions agree across a group in practice, so the
// footer reads the first build's.
function DownloadGroup({
  label,
  builds,
  variant,
  hint,
}: {
  label: string | null;
  builds: Build[];
  variant?: "outline";
  hint?: string;
}) {
  if (builds.length === 0) return null;
  const multi = builds.length > 1;
  return (
    <div className="flex flex-col gap-2">
      {label && (
        <p className="font-mono text-[10px] font-medium uppercase tracking-[0.18em] text-ink-subtle">{label}</p>
      )}
      {builds.map((b) => (
        <a
          key={b.url}
          href={b.url}
          download
          className={cn(buttonVariants(variant ? { variant } : undefined), "w-full")}
        >
          <Download size={16} />
          Download{multi ? ` (${b.arch})` : ""}
        </a>
      ))}
      <p className="text-center font-mono text-[11px] text-ink-subtle">
        {fileKind(builds[0].file)}
        {multi ? "" : ` · ${formatSize(builds[0].size)}`}
        {builds[0].version ? ` · v${builds[0].version}` : ""}
      </p>
      {hint && (
        <p className="rounded-md border border-border bg-surface/60 px-2.5 py-2 text-center font-mono text-[11px] leading-5 text-ink-muted">
          {hint}
        </p>
      )}
    </div>
  );
}

function byArch(builds: Build[]): Build[] {
  return [...builds].sort((a, b) => a.arch.localeCompare(b.arch));
}

// Manifests published before the app/cli split have no `kind`. Read those as
// apps: the page ships ahead of the artifacts (the frontend is embedded in the
// server binary, the builds land on the downloads volume separately), so for the
// window between deploying this and publishing the next release the manifest on
// disk is still the old one — and dropping those builds would empty the page.
function kindOf(b: Build): Build["kind"] {
  return b.kind ?? "app";
}

// The browser client has nothing to download — a tab on any machine becomes the
// device once you create one — so it links into the console instead.
function BrowserCard() {
  return (
    <Card className="flex flex-col p-5">
      <span className="flex h-9 w-9 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
        <Globe size={17} />
      </span>
      <h2 className="mt-3.5 font-display text-[15px] font-bold text-ink">Browser</h2>
      <p className="mt-1 font-mono text-[11px] uppercase tracking-[0.14em] text-ink-subtle">Any modern browser</p>
      <p className="mt-2.5 text-sm leading-6 text-ink-muted">
        Nothing to install. Add a browser device and open its link — that tab is the device your agent drives.
      </p>

      <div className="mt-5 flex-1" />

      <Link to="/login" className={cn(buttonVariants({ variant: "outline" }), "w-full")}>
        <Globe size={16} />
        Create in console
      </Link>
    </Card>
  );
}

function Step({ n, title, body }: { n: number; title: string; body: string }) {
  return (
    <li className="rounded-[10px] border border-border bg-surface/80 p-5 backdrop-blur">
      <span className="flex h-7 w-7 items-center justify-center rounded-md border border-brand/25 bg-brand-soft font-display text-[13px] font-bold text-brand">
        {n}
      </span>
      <h3 className="mt-3 font-display text-[15px] font-bold text-ink">{title}</h3>
      <p className="mt-1.5 text-sm leading-6 text-ink-muted">{body}</p>
    </li>
  );
}

// "abacad-0.4.0-macos-apple-silicon.dmg" -> "DMG". The extension is the most useful label
// for an artifact whose name is otherwise the same on every platform.
function fileKind(file: string): string {
  if (file.endsWith(".tar.gz")) return "TAR.GZ";
  const dot = file.lastIndexOf(".");
  return dot === -1 ? "FILE" : file.slice(dot + 1).toUpperCase();
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const mb = bytes / (1024 * 1024);
  if (mb < 1) return `${Math.round(bytes / 1024)} KB`;
  return `${mb.toFixed(1)} MB`;
}
