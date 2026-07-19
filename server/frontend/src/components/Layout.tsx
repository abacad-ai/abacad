import { Link, useLocation, useNavigate } from "react-router-dom";
import { LogOut, MonitorSmartphone } from "lucide-react";
import { type ReactNode } from "react";
import { api } from "@/lib/api";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";
import { ThemeToggle } from "@/components/ThemeToggle";

const navigation = [
  { to: "/", label: "Devices" },
  { to: "/activities", label: "Activities" },
  { to: "/settings", label: "Settings" },
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
    <div className="min-h-dvh bg-canvas text-ink">
      <a href="#main-content" className="skip-link">
        Skip to content
      </a>

      <header className="sticky top-0 z-40 border-b border-border bg-canvas/85 backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-3 px-4 sm:gap-5 sm:px-6">
          <Brand />

          <nav
            aria-label="Primary navigation"
            className="flex items-center gap-1 rounded-full border border-border bg-surface p-1"
        >
            {navigation.map((item) => {
              const active = loc.pathname === item.to;
              return (
                <Link
                  key={item.to}
                  to={item.to}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "flex h-8 items-center rounded-full px-3 text-[13px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand sm:px-3.5",
                    active ? "bg-brand-soft text-brand" : "text-ink-muted hover:text-ink",
                  )}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>

          <div className="ml-auto flex items-center gap-2">
            <ThemeToggle />
            {me && (
              <>
                <span
                  className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-brand-soft font-display text-[13px] font-bold text-brand"
                  title={me.email}
                >
                  {me.email.slice(0, 1).toUpperCase()}
                </span>
                <span className="hidden max-w-44 truncate text-[13px] text-ink-muted md:block">{me.email}</span>
                <button
                  type="button"
                  onClick={logout}
                  className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  title="Sign out"
                  aria-label="Sign out"
                >
                  <LogOut size={17} />
                </button>
              </>
            )}
          </div>
        </div>
      </header>

      <main id="main-content" tabIndex={-1} className="mx-auto w-full max-w-6xl px-4 pb-16 pt-8 outline-none sm:px-6 sm:pt-10">
        {children}
      </main>
    </div>
  );
}

function Brand() {
  return (
    <Link
      to="/"
      className="flex min-w-0 shrink-0 items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
    >
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-brand/30 bg-brand-soft text-brand">
        <MonitorSmartphone size={17} strokeWidth={2.2} />
      </span>
      <span className="hidden min-[420px]:block">
        <span className="block font-display text-[15px] font-bold uppercase leading-4 tracking-[0.22em] text-ink">
          abacad
        </span>
        <span className="block font-mono text-[10px] uppercase leading-4 tracking-[0.14em] text-ink-subtle">
          device relay
        </span>
      </span>
    </Link>
  );
}
