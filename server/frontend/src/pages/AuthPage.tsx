import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, ApiError } from "@/lib/api";
import { useAuth } from "@/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

export function AuthPage() {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { setMe } = useAuth();
  const nav = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const me = mode === "login" ? await api.login(email, password) : await api.register(email, password);
      setMe(me);
      nav("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-full max-w-sm flex-col justify-center px-4 py-16">
      <div className="mb-8 flex flex-col items-center gap-3">
        <img src="/icon.svg" alt="" className="h-12 w-12 rounded-xl bg-white p-1" />
        <h1 className="text-2xl font-bold tracking-tight">Abacad</h1>
        <p className="text-sm text-slate-400">A device interface for agents.</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{mode === "login" ? "Sign in" : "Create account"}</CardTitle>
          <CardDescription>
            {mode === "login" ? "Access your devices and MCP endpoint." : "Start connecting devices to your agents."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="email">Email</Label>
              <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                required
                minLength={6}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {error && <p className="text-sm text-red-400">{error}</p>}
            <Button type="submit" disabled={busy}>
              {busy ? "…" : mode === "login" ? "Sign in" : "Create account"}
            </Button>
          </form>
          <p className="mt-4 text-center text-sm text-slate-400">
            {mode === "login" ? "No account?" : "Already have one?"}{" "}
            <button
              className="font-medium text-sky-400 hover:underline"
              onClick={() => {
                setMode(mode === "login" ? "register" : "login");
                setError(null);
              }}
            >
              {mode === "login" ? "Create one" : "Sign in"}
            </button>
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
