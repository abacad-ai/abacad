import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowRight, Eye, EyeOff, LoaderCircle, MoveRight } from "lucide-react";
import { RelayMark } from "@/components/RelayMark";
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
  const [googleEnabled, setGoogleEnabled] = useState(false);
  const { setMe } = useAuth();
  const nav = useNavigate();

  // Ask the server which sign-in methods are available, and surface any error the
  // Google callback bounced back via ?error= (e.g. cancelled or expired flow).
  useEffect(() => {
    api
      .authConfig()
      .then((c) => setGoogleEnabled(c.google))
      .catch(() => {});
    const oauthError = new URLSearchParams(window.location.search).get("error");
    if (oauthError) setError(oauthError);
  }, []);

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
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-canvas px-4 py-12 text-ink">
      <div className="bg-grid pointer-events-none absolute inset-0" aria-hidden="true" />
      <div className="glow-brand pointer-events-none absolute inset-0" aria-hidden="true" />

      <div className="relative w-full max-w-[420px]">
        <div className="mb-9 flex flex-col items-center text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-lg border border-brand/30 bg-brand-soft text-brand">
            <RelayMark className="h-8 w-8" />
          </span>
          <p className="mt-4 font-display text-[26px] font-bold uppercase leading-8 tracking-[0.28em] text-ink">
            abacad
          </p>
          <p className="mt-1.5 font-mono text-[11px] uppercase tracking-[0.22em] text-ink-subtle">
            device relay console
          </p>
        </div>

        <div className="rounded-[10px] border border-border bg-surface p-6 shadow-[0_24px_64px_var(--shadow-strong)] sm:p-7">
          <div className="mb-6">
            <h1 className="font-display text-xl font-bold text-ink">
              {mode === "login" ? "Sign in" : "Create account"}
            </h1>
            <p className="mt-1.5 text-sm leading-6 text-ink-muted">
              {mode === "login"
                ? "Manage paired devices and your agent connection."
                : "Set up an account, then pair your first device."}
            </p>
          </div>

          <div className="mb-6 grid grid-cols-2 rounded-md border border-border bg-canvas p-1" aria-label="Authentication mode">
            <button
              type="button"
              onClick={() => switchMode("login")}
              className={cn(
                "h-9 rounded-[4px] text-[13px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                mode === "login" ? "bg-surface-raised text-ink" : "text-ink-muted hover:text-ink",
              )}
              aria-pressed={mode === "login"}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => switchMode("register")}
              className={cn(
                "h-9 rounded-[4px] text-[13px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand",
                mode === "register" ? "bg-surface-raised text-ink" : "text-ink-muted hover:text-ink",
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

          {googleEnabled && (
            <>
              <div className="my-6 flex items-center gap-3" aria-hidden="true">
                <span className="h-px flex-1 bg-border" />
                <span className="font-mono text-[11px] uppercase tracking-[0.18em] text-ink-subtle">or</span>
                <span className="h-px flex-1 bg-border" />
              </div>
              <button
                type="button"
                onClick={() => {
                  // Full-page navigation: the OAuth flow is a browser redirect to
                  // Google and back, not an XHR.
                  window.location.href = "/api/auth/google/start";
                }}
                className="flex h-11 w-full items-center justify-center gap-3 rounded-md border border-border bg-surface-raised text-[13px] font-semibold text-ink transition-colors hover:bg-canvas focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <GoogleIcon className="h-[18px] w-[18px]" />
                Continue with Google
              </button>
            </>
          )}

          <p className="mt-6 text-center text-sm text-ink-muted">
            {mode === "login" ? "New to abacad?" : "Already have an account?"}{" "}
            <button
              type="button"
              className="min-h-11 rounded-md px-1 font-semibold text-brand hover:text-brand-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              onClick={() => switchMode(mode === "login" ? "register" : "login")}
            >
              {mode === "login" ? "Create an account" : "Sign in"}
            </button>
          </p>
        </div>

        <div className="mt-8 flex items-center justify-center gap-2 font-mono text-[11px] uppercase tracking-[0.14em]">
          <span className="rounded border border-border bg-surface px-2.5 py-1.5 text-ink-muted">agent</span>
          <MoveRight size={15} className="shrink-0 text-brand" aria-hidden="true" />
          <span className="rounded border border-brand/25 bg-brand-soft px-2.5 py-1.5 text-brand">relay</span>
          <MoveRight size={15} className="shrink-0 text-brand" aria-hidden="true" />
          <span className="rounded border border-border bg-surface px-2.5 py-1.5 text-ink-muted">devices</span>
        </div>
        <p className="mt-4 text-center font-mono text-[10px] uppercase tracking-[0.2em] text-ink-subtle">
          tokens hashed · secrets shown once
        </p>
      </div>
    </main>
  );
}

// GoogleIcon is the multi-color Google "G". Inlined because lucide (this app's
// icon set) ships no brand marks; the fixed brand colors are intentional.
function GoogleIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 18 18" aria-hidden="true">
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.92c1.7-1.57 2.68-3.88 2.68-6.62Z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.92-2.26c-.8.54-1.84.86-3.04.86-2.34 0-4.32-1.58-5.02-3.7H.96v2.34A9 9 0 0 0 9 18Z"
      />
      <path
        fill="#FBBC05"
        d="M3.98 10.72a5.4 5.4 0 0 1 0-3.44V4.94H.96a9 9 0 0 0 0 8.12l3.02-2.34Z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.32 0 2.5.46 3.44 1.35l2.58-2.58C13.46.9 11.43 0 9 0A9 9 0 0 0 .96 4.94l3.02 2.34C4.68 5.16 6.66 3.58 9 3.58Z"
      />
    </svg>
  );
}
