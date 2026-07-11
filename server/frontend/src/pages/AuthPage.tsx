import { useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowRight,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  MonitorSmartphone,
  Radio,
  ShieldCheck,
} from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

export function AuthPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { setMe } = useAuth();
  const nav = useNavigate();

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const me = mode === "login" ? await api.login(email, password) : await api.register(email, password);
      setMe(me);
      nav("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Unable to continue. Check your connection and try again.");
    } finally {
      setBusy(false);
    }
  };

  const switchMode = (next: "login" | "register") => {
    setMode(next);
    setError(null);
  };

  return (
    <main className="grid min-h-dvh bg-canvas text-ink lg:grid-cols-[minmax(0,1.05fr)_minmax(440px,0.95fr)]">
      <section className="relative hidden min-h-dvh overflow-hidden border-r border-border bg-sidebar p-12 lg:flex lg:flex-col">
        <div className="flex items-center gap-3">
          <span className="flex h-10 w-10 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
            <MonitorSmartphone size={21} strokeWidth={2.2} />
          </span>
          <div>
            <p className="text-base font-bold leading-5">Abacad</p>
            <p className="text-xs text-ink-subtle">Device relay</p>
          </div>
        </div>

        <div className="my-auto max-w-xl py-16">
          <p className="mb-4 text-sm font-semibold text-brand">Remote control plane</p>
          <h1 className="max-w-lg text-4xl font-semibold leading-[1.12] text-ink">
            Your agents, connected to the devices that do the work.
          </h1>
          <p className="mt-5 max-w-lg text-base leading-7 text-ink-muted">
            Pair phones and machines once, then route agent commands through a single authenticated MCP endpoint.
          </p>

          <div className="mt-10 max-w-lg rounded-lg border border-border bg-canvas/60 p-4">
            <ConnectionRow icon={KeyRound} label="Agent" detail="MCP over HTTPS" active />
            <div className="ml-[19px] h-5 border-l border-dashed border-border-strong" />
            <ConnectionRow icon={Radio} label="Abacad relay" detail="Authenticated routing" active />
            <div className="ml-[19px] h-5 border-l border-dashed border-border-strong" />
            <ConnectionRow icon={MonitorSmartphone} label="Your devices" detail="Outbound secure connection" />
          </div>
        </div>

        <div className="flex items-center gap-2 text-xs text-ink-subtle">
          <ShieldCheck size={16} className="text-brand" />
          Tokens are stored as secure hashes and secrets are shown once.
        </div>
      </section>

      <section className="flex min-h-dvh items-center justify-center px-4 py-10 sm:px-8 lg:px-12">
        <div className="w-full max-w-[420px]">
          <div className="mb-9 flex items-center gap-3 lg:hidden">
            <span className="flex h-10 w-10 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
              <MonitorSmartphone size={21} strokeWidth={2.2} />
            </span>
            <div>
              <p className="font-bold leading-5">Abacad</p>
              <p className="text-xs text-ink-subtle">Device relay</p>
            </div>
          </div>

          <div className="mb-7">
            <h1 className="text-2xl font-semibold text-ink">
              {mode === "login" ? "Sign in to your workspace" : "Create your workspace"}
            </h1>
            <p className="mt-2 text-sm leading-6 text-ink-muted">
              {mode === "login"
                ? "Manage paired devices and your agent connection."
                : "Set up an account, then pair your first device."}
            </p>
          </div>

          <div className="mb-6 grid grid-cols-2 rounded-md border border-border bg-sidebar p-1" aria-label="Authentication mode">
            <button
              type="button"
              onClick={() => switchMode("login")}
              className={cn(
                "h-10 rounded-[4px] text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                mode === "login" ? "bg-surface-raised text-ink shadow-sm" : "text-ink-muted hover:text-ink",
              )}
              aria-pressed={mode === "login"}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => switchMode("register")}
              className={cn(
                "h-10 rounded-[4px] text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                mode === "register" ? "bg-surface-raised text-ink shadow-sm" : "text-ink-muted hover:text-ink",
              )}
              aria-pressed={mode === "register"}
            >
              Create account
            </button>
          </div>

          <form onSubmit={submit} className="flex flex-col gap-5">
            <div className="flex flex-col gap-2">
              <Label htmlFor="email">Email address</Label>
              <Input
                id="email"
                type="email"
                autoComplete="email"
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@example.com"
              />
            </div>

            <div className="flex flex-col gap-2">
              <Label htmlFor="password">Password</Label>
              <div className="relative">
                <Input
                  id="password"
                  type={showPassword ? "text" : "password"}
                  autoComplete={mode === "login" ? "current-password" : "new-password"}
                  required
                  minLength={6}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="pr-12"
                  aria-describedby={mode === "register" ? "password-help" : undefined}
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((shown) => !shown)}
                  className="absolute right-0 top-0 flex h-11 w-11 items-center justify-center rounded-md text-ink-subtle transition-colors hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  aria-label={showPassword ? "Hide password" : "Show password"}
                >
                  {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </div>
              {mode === "register" && (
                <p id="password-help" className="text-xs text-ink-subtle">
                  Use at least 6 characters.
                </p>
              )}
            </div>

            {error && (
              <div role="alert" className="rounded-md border border-danger/25 bg-danger-soft px-3.5 py-3 text-sm text-danger">
                {error}
              </div>
            )}

            <Button type="submit" disabled={busy} className="mt-1 w-full">
              {busy ? (
                <>
                  <LoaderCircle size={17} className="animate-spin" />
                  {mode === "login" ? "Signing in" : "Creating account"}
                </>
              ) : (
                <>
                  {mode === "login" ? "Sign in" : "Create account"}
                  <ArrowRight size={17} />
                </>
              )}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-ink-muted">
            {mode === "login" ? "New to Abacad?" : "Already have an account?"}{" "}
            <button
              type="button"
              className="min-h-11 rounded-md px-1 font-semibold text-brand hover:text-brand-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              onClick={() => switchMode(mode === "login" ? "register" : "login")}
            >
              {mode === "login" ? "Create an account" : "Sign in"}
            </button>
          </p>
        </div>
      </section>
    </main>
  );
}

function ConnectionRow({
  icon: Icon,
  label,
  detail,
  active = false,
}: {
  icon: typeof KeyRound;
  label: string;
  detail: string;
  active?: boolean;
}) {
  return (
    <div className="flex items-center gap-3">
      <span className={cn("flex h-10 w-10 items-center justify-center rounded-md border", active ? "border-brand/25 bg-brand-soft text-brand" : "border-border bg-surface text-ink-muted")}>
        <Icon size={18} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-ink">{label}</p>
        <p className="text-xs text-ink-subtle">{detail}</p>
      </div>
      <span className={cn("h-2 w-2 rounded-full", active ? "bg-success" : "bg-ink-subtle")} aria-hidden="true" />
    </div>
  );
}
