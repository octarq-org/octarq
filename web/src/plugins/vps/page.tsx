import { useEffect, useRef, useState } from "react";
import { api, VPS, SSHKey } from "../../api";
import { Empty, Field, Modal, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, LockedFeature } from "@octarq-org/plugin-sdk";
import { Terminal as XTerminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { Server, Key, Cpu, Terminal, Pencil, Trash2 } from "lucide-react";
import { useTranslation } from "@octarq-org/plugin-sdk";
import "@xterm/xterm/css/xterm.css";

export default function VPSPage() {
  const [list, setList] = useState<VPS[]>([]);
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [showAdd, setShowAdd] = useState(false);
  const [editItem, setEditItem] = useState<VPS | null>(null);
  const [terminalVPS, setTerminalVPS] = useState<VPS | null>(null);
  const [error, setError] = useState<{ status: number } | null>(null);
  const { t } = useTranslation();

  function load() {
    api.vpsList()
      .then(setList)
      .catch((err) => setError({ status: err.status }));
    api.sshKeys().then(setKeys).catch(() => {});
  }

  useEffect(() => {
    load();
    const timer = setInterval(() => {
      if (!error) load();
    }, 30000); // refresh status every 30s
    return () => clearInterval(timer);
  }, [error]);

  if (error) {
    return (
      <ScreenWrap>
        <LockedFeature
          status={error.status}
          tier="pro"
          feature={t("vps.feature")}
          description={t("vps.lockedDesc")}
          perks={[
            t("vps.perkMonitoring"),
            t("vps.perkVault"),
            t("vps.perkTerminal"),
          ]}
          icon={<Server className="h-7 w-7" />}
          pricingHref="https://octarq.com/pricing/"
        />
      </ScreenWrap>
    );
  }

  return (
    <ScreenWrap>
      <PageHeader
        title={t("vps.pageTitle")}
        description={t("vps.pageDesc")}
        action={
          <Button variant="primary" onClick={() => setShowAdd(true)}>
            {t("vps.addVps")}
          </Button>
        }
      />

      {list.length === 0 ? (
        <Empty>
          <Server className="h-10 w-10 text-white/50 mb-2" />
          <p className="text-sm text-white/50">{t("vps.emptyText")}</p>
          <Button variant="primary" className="mt-4" onClick={() => setShowAdd(true)}>
            {t("vps.emptyAdd")}
          </Button>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
          {list.map((vps) => {
            const statusTone = vps.status === "online" ? "green" : vps.status === "offline" ? "red" : "neutral";
            const keyName = keys.find(k => k.id === vps.sshKeyId)?.name || t("vps.unknownKey");

            return (
              <GlassCard key={vps.id} className="p-5 flex flex-col sm:flex-row gap-5 items-start sm:items-center">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2.5 mb-2">
                    <span className={`h-2.5 w-2.5 rounded-full shrink-0 ${
                      vps.status === "online" ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.4)]" :
                      vps.status === "offline" ? "bg-rose-500 shadow-[0_0_8px_rgba(244,63,94,0.4)]" :
                      "bg-white/20"
                    }`} />
                    <h3 className="font-semibold text-base truncate text-white">{vps.name}</h3>
                    <Badge tone={statusTone} className="capitalize text-[10px]">
                      {vps.status === "online" ? t("vps.statusOnline") : vps.status === "offline" ? t("vps.statusOffline") : vps.status}
                    </Badge>
                  </div>
                  
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-x-4 gap-y-1.5 text-xs text-white/55 mt-3">
                    <div className="flex items-center gap-1.5 min-w-0">
                      <span className="text-white/50">{t("vps.ipLabel")}</span>{" "}
                      <span className="font-mono truncate">{vps.ip}:{vps.port}</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <span className="text-white/50">{t("vps.userLabel")}</span>{" "}
                      <span className="font-medium">{vps.user}</span>
                    </div>
                    <div className="flex items-center gap-1.5 min-w-0 sm:col-span-2">
                      <Key className="h-3.5 w-3.5 text-white/50 shrink-0" />
                      <span className="text-white/50">{t("vps.keyLabel")}</span>{" "}
                      <span className="truncate font-medium text-white/70">{keyName}</span>
                    </div>
                  </div>
                  
                  <div className="text-[11px] text-white/50 mt-4 border-t border-white/[0.04] pt-3">
                    {vps.lastChecked ? t("vps.lastActive", { time: timeAgo(vps.lastChecked) }) : t("vps.pendingTest")}
                    {vps.failCount > 0 && vps.status !== "online" && ` ${t("vps.failedAttempts", { count: vps.failCount })}`}
                  </div>
                </div>
                
                <div className="flex sm:flex-col gap-2 w-full sm:w-auto shrink-0 border-t sm:border-t-0 border-white/[0.06] pt-4 sm:pt-0">
                  <Button 
                    variant="primary"
                    onClick={() => setTerminalVPS(vps)}
                    className="flex-1 sm:flex-none py-1.5 text-xs gap-1.5"
                  >
                    <Terminal className="h-3.5 w-3.5" />
                    {t("vps.terminal")}
                  </Button>
                  <div className="flex sm:flex-row gap-2 w-full">
                    <Button 
                      variant="subtle"
                      onClick={() => setEditItem(vps)}
                      className="flex-1 py-1.5 text-xs gap-1"
                    >
                      <Pencil className="h-3 w-3" />
                      {t("vps.edit")}
                    </Button>
                    <Button 
                      variant="danger"
                      onClick={async () => {
                        if (!confirm(t("vps.confirmRemove", { name: vps.name }))) return;
                        await api.deleteVPS(vps.id);
                        load();
                      }}
                      className="flex-1 py-1.5 text-xs gap-1 bg-rose-500/10 hover:bg-rose-500/25"
                    >
                      <Trash2 className="h-3 w-3" />
                      {t("vps.remove")}
                    </Button>
                  </div>
                </div>
              </GlassCard>
            );
          })}
        </div>
      )}

      {(showAdd || editItem) && (
        <VPSModal 
          keys={keys}
          vps={editItem}
          onClose={() => { setShowAdd(false); setEditItem(null); }} 
          onSaved={load} 
        />
      )}

      {terminalVPS && (
        <TerminalModal vps={terminalVPS} onClose={() => setTerminalVPS(null)} />
      )}
    </ScreenWrap>
  );
}

function VPSModal({ keys, vps, onClose, onSaved }: { keys: SSHKey[]; vps: VPS | null; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(vps?.name || "");
  const [ip, setIp] = useState(vps?.ip || "");
  const [port, setPort] = useState(vps?.port?.toString() || "22");
  const [user, setUser] = useState(vps?.user || "root");
  const [sshKeyId, setSshKeyId] = useState(vps?.sshKeyId?.toString() || (keys[0]?.id.toString() || ""));
  
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const { t } = useTranslation();

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const p = {
        name,
        ip,
        port: parseInt(port, 10),
        user,
        sshKeyId: parseInt(sshKeyId, 10),
      };
      if (vps) {
        await api.updateVPS(vps.id, p);
      } else {
        await api.createVPS(p);
      }
      onSaved();
      onClose();
    } catch (e: any) {
      setErr(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={vps ? t("vps.editServer") : t("vps.registerServer")} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={t("vps.nameLabel")}>
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required autoFocus placeholder={t("vps.namePlaceholder")} />
        </Field>
        
        <div className="flex gap-4">
          <div className="flex-[3]">
            <Field label={t("vps.ipHostLabel")}>
              <input className="input w-full font-mono" value={ip} onChange={(e) => setIp(e.target.value)} required placeholder="192.168.1.1" />
            </Field>
          </div>
          <div className="flex-1">
            <Field label={t("vps.portLabel")}>
              <input className="input w-full font-mono" type="number" min="1" max="65535" value={port} onChange={(e) => setPort(e.target.value)} required />
            </Field>
          </div>
        </div>

        <Field label={t("vps.usernameLabel")}>
          <input className="input w-full font-mono" value={user} onChange={(e) => setUser(e.target.value)} required />
        </Field>

        <Field label={t("vps.sshKeyLabel")} hint={t("vps.sshKeyHint")}>
          <select 
            className="input w-full" 
            value={sshKeyId} 
            onChange={(e) => setSshKeyId(e.target.value)}
            required
          >
            <option value="" disabled>{t("vps.selectKey")}</option>
            {keys.map((k) => (
              <option key={k.id} value={k.id.toString()}>{k.name} ({k.type})</option>
            ))}
          </select>
        </Field>

        {err && <p className="text-sm text-red-400 font-medium">{err}</p>}
        
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>{t("vps.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name || !ip || !sshKeyId}>
            {busy ? t("vps.saving") : t("vps.saveConfig")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function TerminalModal({ vps, onClose }: { vps: VPS; onClose: () => void }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  
  const [status, setStatus] = useState("Connecting...");
  const { t } = useTranslation();

  useEffect(() => {
    if (!containerRef.current) return;
    
    const term = new XTerminal({
      cursorBlink: true,
      theme: {
        background: '#09090b', // zinc-950
        foreground: '#e4e4e7', // zinc-200
        black: '#18181b',
        red: '#ef4444',
        green: '#22c55e',
        yellow: '#eab308',
        blue: '#3b82f6',
        magenta: '#d946ef',
        cyan: '#06b6d4',
        white: '#f4f4f5',
        brightBlack: '#52525b',
        brightRed: '#f87171',
        brightGreen: '#4ade80',
        brightYellow: '#fde047',
        brightBlue: '#60a5fa',
        brightMagenta: '#e879f9',
        brightCyan: '#22d3ee',
        brightWhite: '#ffffff',
      },
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      fontSize: 14,
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    
    term.open(containerRef.current);
    fitAddon.fit();
    termRef.current = term;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/api/vps/${vps.id}/terminal`;
    
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setStatus("Connected");
      // Notify backend of terminal size
      ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
    };

    ws.onmessage = (ev) => {
      try {
        const msg = JSON.parse(ev.data);
        if (msg.type === "data") {
          term.write(msg.data);
        }
      } catch (e) {
        console.error("ws parse error", e);
      }
    };

    ws.onclose = () => {
      setStatus("Disconnected");
      term.write(`\r\n\x1b[31m${t("vps.connectionClosed")}\x1b[m\r\n`);
    };
    
    ws.onerror = () => {
      setStatus("Error");
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "data", data }));
      }
    });

    const resizeObserver = new ResizeObserver(() => {
      try {
        fitAddon.fit();
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
        }
      } catch (e) {}
    });
    resizeObserver.observe(containerRef.current);

    return () => {
      resizeObserver.disconnect();
      ws.close();
      term.dispose();
    };
  }, [vps]);

  return (
    <div className="fixed inset-0 z-[100] flex flex-col bg-[#07070b]">
      <div className="flex items-center justify-between px-4 py-2 bg-white/[0.04] border-b border-white/[0.06]">
        <div className="flex items-center gap-3">
          <span className="font-semibold text-white/90 text-sm font-mono">{vps.user}@{vps.name}</span>
          <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${
            status === "Connected" ? "bg-green-500/20 text-green-400" :
            status === "Connecting..." ? "bg-yellow-500/20 text-yellow-400" :
            "bg-red-500/20 text-red-400"
          }`}>
            {status === "Connecting..." ? t("vps.termConnecting") :
             status === "Connected" ? t("vps.termConnected") :
             status === "Disconnected" ? t("vps.termDisconnected") :
             status === "Error" ? t("vps.termError") : status}
          </span>
        </div>
        <Button 
          variant="ghost" 
          onClick={onClose}
          className="text-xs py-1 px-2.5"
        >
          {t("vps.closeTerminal")}
        </Button>
      </div>
      <div className="flex-1 w-full relative">
        <div ref={containerRef} className="absolute inset-0 p-2" />
      </div>
    </div>
  );
}
