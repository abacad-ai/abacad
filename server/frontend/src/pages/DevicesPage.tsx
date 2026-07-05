import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Plus, RefreshCw, Trash2, Pencil } from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

interface Reveal {
  title: string;
  wssUrl: string;
  token: string;
}

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);

  const reload = async () => {
    try {
      setDevices(await api.devices());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, []);

  const addDevice = async () => {
    const name = window.prompt("Name this device", "My phone");
    if (name === null) return;
    const d = await api.createDevice(name || "New device");
    setReveal({ title: `Connect “${d.name}”`, wssUrl: d.wss_url, token: d.device_token });
    void reload();
  };

  const rename = async (d: DeviceView) => {
    const name = window.prompt("Rename device", d.name);
    if (!name || name === d.name) return;
    await api.renameDevice(d.id, name);
    void reload();
  };

  const remove = async (d: DeviceView) => {
    if (!window.confirm(`Remove “${d.name}”? Its token stops working immediately.`)) return;
    await api.deleteDevice(d.id);
    void reload();
  };

  const rotate = async (d: DeviceView) => {
    if (!window.confirm(`Rotate the token for “${d.name}”? The current one stops working.`)) return;
    const r = await api.rotateDeviceToken(d.id);
    setReveal({ title: `New token for “${d.name}”`, wssUrl: r.wss_url, token: r.device_token });
  };

  return (
    <div>
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">Devices</h1>
          <p className="text-sm text-slate-400">Phones and machines your agents can drive.</p>
        </div>
        <Button onClick={addDevice}>
          <Plus size={16} /> Add device
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : error ? (
        <p className="text-sm text-red-400">{error}</p>
      ) : devices.length === 0 ? (
        <Card className="p-8 text-center">
          <p className="text-sm text-slate-400">No devices yet.</p>
          <p className="mt-1 text-sm text-slate-500">Add one, then paste its URL into the Abacad app.</p>
        </Card>
      ) : (
        <div className="flex flex-col gap-2.5">
          {devices.map((d) => (
            <Card key={d.id} className="flex items-center justify-between p-4">
              <div className="flex items-center gap-3">
                <span
                  className={`h-2.5 w-2.5 rounded-full ${d.online ? "bg-emerald-400" : "bg-slate-600"}`}
                  title={d.online ? "online" : "offline"}
                />
                <div>
                  <div className="font-medium text-slate-100">{d.name}</div>
                  <div className="font-mono text-xs text-slate-500">{d.id}</div>
                </div>
              </div>
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="icon" onClick={() => rename(d)} title="Rename">
                  <Pencil size={15} />
                </Button>
                <Button variant="ghost" size="icon" onClick={() => rotate(d)} title="Rotate token">
                  <RefreshCw size={15} />
                </Button>
                <Button variant="ghost" size="icon" onClick={() => remove(d)} title="Remove">
                  <Trash2 size={15} />
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}

      <Modal open={reveal !== null} onClose={() => setReveal(null)} title={reveal?.title ?? ""}>
        {reveal && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-slate-400">
              Paste this URL into the Abacad app on the device, or scan the QR. Shown once — copy it now.
            </p>
            <div className="flex justify-center rounded-xl bg-white p-4">
              <QRCodeSVG value={reveal.wssUrl} size={168} />
            </div>
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Connection URL</div>
              <CopyField value={reveal.wssUrl} />
            </div>
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Device token</div>
              <CopyField value={reveal.token} />
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}
