import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Download, Globe, Laptop, LoaderCircle, Monitor, Smartphone, Terminal } from "lucide-react";
import { api, type ClientBuild } from "@/lib/api";
import { PublicLayout } from "@/components/PublicLayout";
import { buttonVariants } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { platformInfo } from "@/lib/devices";
import { cn, relativeTime } from "@/lib/utils";

// Public client downloads at /downloads — deliberately reachable with no account,
// since you install the client on a device before (or while) signing up. The
// buttons come from GET /api/downloads, so the page offers exactly the builds
// that exist on the server: no dead links, and a newly published platform shows
// up without a frontend change.
//
// Note the backend also owns /downloads/<file> (the artifacts themselves); this
// SPA route is the bare /downloads path, which Go's mux leaves to the SPA.

interface PlatformCard {
  key: string; // matches ClientBuild.platform
  label: string;
  icon: typeof Laptop;
  requirement: string;
  note: string;
}

// The clients we describe, in display order. A platform with no published build
// is still listed (as "coming soon") — knowing it is planned beats a blank page.
const CATALOG: PlatformCard[] = [
  {
    key: "macos",
    label: "macOS",
    icon: Laptop,
    requirement: "macOS 14 Sonoma or later",
    note: "Grant Accessibility and Screen Recording on first launch, then it runs from the menu bar.",
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
    requirement: "Windows 11",
    note: "In development.",
  },
  {
    key: "linux",
    label: "Linux",
    icon: Terminal,
    requirement: "x86_64",
    note: "In development.",
  },
];

export function DownloadsPage() {
  const [builds, setBuilds] = useState<ClientBuild[] | null>(null);
  const [failed, setFailed] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .downloads()
      .then((list) => live && setBuilds(list))
      .catch((err: Error) => live && setFailed(err.message));
    return () => {
      live = false;
    };
  }, []);

  const buildFor = (key: string) => builds?.find((b) => b.platform === key) ?? null;
  // A build published for a platform the catalog doesn't describe still gets a
  // card, so copying a file to the server is always enough to offer it.
  const extras = (builds ?? []).filter((b) => !CATALOG.some((c) => c.key === b.platform));

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
            <ClientCard key={card.key} card={card} build={buildFor(card.key)} loading={builds === null && !failed} />
          ))}
          {extras.map((build) => (
            <ClientCard
              key={build.platform}
              card={{
                key: build.platform,
                label: platformInfo(build.platform).label,
                icon: Laptop,
                requirement: "Latest build",
                note: "",
              }}
              build={build}
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

function ClientCard({
  card,
  build,
  loading,
}: {
  card: PlatformCard;
  build: ClientBuild | null;
  loading: boolean;
}) {
  const Icon = card.icon;
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
      ) : build ? (
        <>
          <a href={build.url} download className={cn(buttonVariants(), "w-full")}>
            <Download size={16} />
            Download
          </a>
          <p className="mt-2 text-center font-mono text-[11px] text-ink-subtle">
            {fileKind(build.file)} · {formatSize(build.size)} · {relativeTime(build.updated_at * 1000)}
          </p>
        </>
      ) : (
        // No published artifact: say so plainly rather than offering a dead button.
        <span className="inline-flex h-11 items-center rounded-md border border-dashed border-border px-3 text-sm text-ink-subtle">
          Not available yet
        </span>
      )}
    </Card>
  );
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

// "abacad-macos-latest.dmg" -> "DMG". The extension is the most useful label for
// an artifact whose name is otherwise the same on every platform.
function fileKind(file: string): string {
  const dot = file.lastIndexOf(".");
  return dot === -1 ? "FILE" : file.slice(dot + 1).toUpperCase();
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const mb = bytes / (1024 * 1024);
  if (mb < 1) return `${Math.round(bytes / 1024)} KB`;
  return `${mb.toFixed(1)} MB`;
}
