import { Link, useLocation, useNavigate } from "react-router-dom";
import { KeyRound, LogOut, MonitorSmartphone, Settings, Smartphone } from "lucide-react";
import { type ReactNode } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

const navigation = [
  { to: "/", icon: Smartphone, label: "Devices" },
  { to: "/settings", icon: Settings, label: "Settings" },
];

export function Layout({ children }: { children: ReactNode }) {
  const { me, setMe } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();

  const logout = async () => {
    await api.logout();
    setMe(null);
    nav("/login");
  };

  return (
    <div className="min-h-dvh bg-canvas text-ink lg:grid lg:grid-cols-[232px_minmax(0,1fr)]">
      <a href="#main-content" className="skip-link">
        Skip to content
      </a>

      <aside className="hidden min-h-dvh border-r border-border bg-sidebar lg:sticky lg:top-0 lg:flex lg:h-dvh lg:flex-col">
        <div className="flex h-20 items-center border-b border-border px-5">
          <Brand />
        </div>

        <nav aria-label="Primary navigation" className="flex flex-1 flex-col gap-1 p-3">
          <p className="px-3 pb-2 pt-3 text-[11px] font-semibold uppercase text-ink-subtle">
            Workspace
          </p>
          {navigation.map((item) => (
            <NavItem key={item.to} {...item} active={loc.pathname === item.to} />
          ))}

          <div className="mt-5 border-t border-border pt-5">
            <div className="mx-2 rounded-lg border border-border bg-surface p-3">
              <div className="flex items-center gap-2 text-xs font-semibold text-ink">
                <KeyRound size={15} className="text-brand" />
                Agent endpoint
              </div>
              <p className="mt-1.5 text-xs leading-5 text-ink-muted">
                One MCP connection routes commands to every paired device.
              </p>
            </div>
          </div>
        </nav>

        {me && (
          <div className="border-t border-border p-3">
            <div className="flex items-center gap-3 px-2 py-2">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-brand-soft text-sm font-bold text-brand">
                {me.email.slice(0, 1).toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-ink">{me.email}</p>
                <p className="text-xs text-ink-subtle">Account owner</p>
              </div>
              <button
                type="button"
                onClick={logout}
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                title="Sign out"
                aria-label="Sign out"
              >
                <LogOut size={18} />
              </button>
            </div>
          </div>
        )}
      </aside>

      <div className="min-w-0">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-border bg-canvas/95 px-4 backdrop-blur sm:px-6 lg:hidden">
          <Brand />
          {me && (
            <button
              type="button"
              onClick={logout}
              className="flex h-11 w-11 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              title="Sign out"
              aria-label="Sign out"
            >
              <LogOut size={18} />
            </button>
          )}
        </header>

        <main id="main-content" tabIndex={-1} className="mx-auto w-full max-w-[1280px] px-4 pb-24 pt-6 outline-none sm:px-6 sm:pt-8 lg:px-8 lg:pb-12 lg:pt-10">
          {children}
        </main>

        <nav
          aria-label="Primary navigation"
          className="fixed inset-x-0 bottom-0 z-40 grid h-[72px] grid-cols-2 border-t border-border bg-sidebar/98 px-4 pb-[env(safe-area-inset-bottom)] backdrop-blur lg:hidden"
        >
          {navigation.map((item) => {
            const Icon = item.icon;
            const active = loc.pathname === item.to;
            return (
              <Link
                key={item.to}
                to={item.to}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "relative flex min-h-14 flex-col items-center justify-center gap-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand",
                  active ? "text-brand" : "text-ink-subtle hover:text-ink",
                )}
              >
                <Icon size={20} strokeWidth={active ? 2.2 : 1.8} />
                {item.label}
                {active && <span className="absolute top-0 h-0.5 w-8 rounded-full bg-brand" />}
              </Link>
            );
          })}
        </nav>
      </div>
    </div>
  );
}

function Brand() {
  return (
    <Link to="/" className="flex min-w-0 items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
        <MonitorSmartphone size={19} strokeWidth={2.2} />
      </span>
      <span>
        <span className="block text-[15px] font-bold leading-4 text-ink">Abacad</span>
        <span className="block text-[11px] leading-4 text-ink-subtle">Device relay</span>
      </span>
    </Link>
  );
}

function NavItem({
  to,
  icon: Icon,
  label,
  active,
}: {
  to: string;
  icon: typeof Smartphone;
  label: string;
  active: boolean;
}) {
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex min-h-11 items-center gap-3 rounded-md px-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
        active ? "bg-brand-soft text-brand" : "text-ink-muted hover:bg-surface-hover hover:text-ink",
      )}
    >
      <Icon size={18} strokeWidth={active ? 2.2 : 1.8} />
      {label}
    </Link>
  );
}
