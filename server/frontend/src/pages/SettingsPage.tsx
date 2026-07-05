import { useEffect, useState } from "react";
import { KeyRound, RefreshCw } from "lucide-react";
import { api, type McpTokenInfo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

interface Revealed {
  token: string;
  url: string;
}

// Derive the MCP endpoint from the browser's location (Vite's /mcp proxy in dev,
// Go in prod) rather than the backend's view of Host, which is "localhost:1213"
// behind the dev proxy and unreachable from another machine on the LAN.
function mcpUrl(): string {
  return `${window.location.protocol}//${window.location.host}/mcp`;
}

export function SettingsPage() {
  const [info, setInfo] = useState<McpTokenInfo | null>(null);
  const [revealed, setRevealed] = useState<Revealed | null>(null);

  const reload = async () => setInfo(await api.mcpToken());
  useEffect(() => {
    void reload();
  }, []);

  const rotate = async () => {
    if (info?.exists && !window.confirm("Generate a new MCP token? The current one stops working.")) return;
    const r = await api.rotateMcpToken();
    setRevealed({ token: r.mcp_token, url: mcpUrl() });
    void reload();
  };

  const cmd = (r: Revealed) =>
    `claude mcp add --transport http abacad ${r.url} --header "Authorization: Bearer ${r.token}"`;

  return (
    <div>
      <h1 className="mb-1 text-xl font-bold">Settings</h1>
      <p className="mb-5 text-sm text-slate-400">Your agent’s connection to Abacad.</p>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <KeyRound size={17} /> MCP token
          </CardTitle>
          <CardDescription>
            One token for your account. Add it to your agent as a bearer header to reach your devices via a single
            MCP endpoint.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {info === null ? (
            <p className="text-sm text-slate-500">Loading…</p>
          ) : info.exists ? (
            <div className="text-sm text-slate-400">
              <div>
                Created <span className="text-slate-300">{fmt(info.created_at)}</span>
              </div>
              <div>
                Last used{" "}
                <span className="text-slate-300">{info.last_used ? fmt(info.last_used) : "never"}</span>
              </div>
            </div>
          ) : (
            <p className="text-sm text-slate-400">No token yet. Generate one to connect your agent.</p>
          )}
          <div>
            <Button variant={info?.exists ? "outline" : "default"} onClick={rotate}>
              <RefreshCw size={15} /> {info?.exists ? "Rotate token" : "Generate token"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Modal open={revealed !== null} onClose={() => setRevealed(null)} title="Your MCP token">
        {revealed && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-slate-400">Shown once — copy it now. Run this to register the endpoint:</p>
            <CopyField value={cmd(revealed)} />
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Token</div>
              <CopyField value={revealed.token} />
            </div>
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Endpoint</div>
              <CopyField value={revealed.url} />
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}

function fmt(iso?: string) {
  if (!iso) return "—";
  return new Date(iso).toLocaleString();
}
