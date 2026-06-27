import { useEffect, useRef, useState } from "react";
import { api, VPS, SSHKey } from "../api";
import { Empty, Field, Modal, timeAgo } from "../ui";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export default function VPSPage() {
  const [list, setList] = useState<VPS[]>([]);
  const [keys, setKeys] = useState<SSHKey[]>([]);
  const [showAdd, setShowAdd] = useState(false);
  const [editItem, setEditItem] = useState<VPS | null>(null);
  const [terminalVPS, setTerminalVPS] = useState<VPS | null>(null);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);

  function load() {
    api.vpsList()
      .then(setList)
      .catch((err) => setError({ status: err.status, message: err.message }));
    api.sshKeys().then(setKeys).catch(() => {});
  }

  useEffect(() => {
    load();
    const t = setInterval(() => {
      if (!error) load();
    }, 30000); // refresh status every 30s
    return () => clearInterval(t);
  }, [error]);

  if (error) {
    return (
      <div className="card flex flex-col items-center justify-center gap-4 py-20 px-6 text-center">
        <div className="text-5xl">{error.status === 402 ? "🔒" : "🔌"}</div>
        <div>
          <h2 className="text-xl font-bold mb-1">
            {error.status === 402 ? "Pro Feature Locked" : "Feature Unavailable"}
          </h2>
          <p className="text-sm text-white/55 max-w-md mx-auto">
            {error.status === 402
              ? "A valid led-pro license is required to manage VPS infrastructure."
              : "The VPS infrastructure feature is not available or disabled in this installation."}
          </p>
        </div>
        {error.status === 402 && (
          <a
            href="/settings/license"
            className="btn-primary mt-2"
          >
            Manage License
          </a>
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-white">VPS Infrastructure</h1>
          <p className="text-sm text-white/55">Manage and monitor your remote servers.</p>
        </div>
        <button className="btn-primary" onClick={() => setShowAdd(true)}>
          + Add VPS
        </button>
      </div>

      {list.length === 0 ? (
        <Empty>
          <div className="text-4xl mb-2">🖥️</div>
          <p>No servers added yet.</p>
          <button className="btn-primary mt-4" onClick={() => setShowAdd(true)}>
            Add VPS
          </button>
        </Empty>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
          {list.map((vps) => (
            <div key={vps.id} className="card p-4 flex flex-col sm:flex-row gap-4 items-start sm:items-center">
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <div className={`w-2.5 h-2.5 rounded-full ${
                    vps.status === "online" ? "bg-green-500 shadow-[0_0_8px_rgba(34,197,94,0.5)]" : 
                    vps.status === "offline" ? "bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]" : 
                    "bg-white/[0.06]"
                  }`} title={vps.status} />
                  <h3 className="font-semibold text-lg truncate">{vps.name}</h3>
                </div>
                
                <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-white/55 mt-2">
                  <div className="flex items-center gap-1">
                    <span className="text-white/40">IP:</span> 
                    <span className="font-mono">{vps.ip}:{vps.port}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <span className="text-white/40">User:</span> 
                    <span>{vps.user}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <span className="text-white/40">Key:</span> 
                    <span className="truncate max-w-xs">{keys.find(k => k.id === vps.sshKeyId)?.name || "?"}</span>
                  </div>
                </div>
                
                <div className="text-xs text-white/40 mt-3">
                  {vps.lastChecked ? `Checked ${timeAgo(vps.lastChecked)}` : "Pending initial check"}
                  {vps.failCount > 0 && vps.status !== "online" && ` (${vps.failCount} fails)`}
                </div>
              </div>
              
              <div className="flex sm:flex-col gap-2 w-full sm:w-auto">
                <button 
                  className="btn-primary flex-1 sm:flex-none justify-center"
                  onClick={() => setTerminalVPS(vps)}
                >
                  Terminal
                </button>
                <div className="flex gap-2 w-full">
                  <button 
                    className="btn-ghost flex-1 sm:flex-none justify-center text-xs"
                    onClick={() => setEditItem(vps)}
                  >
                    Edit
                  </button>
                  <button 
                    className="btn-ghost flex-1 sm:flex-none justify-center text-xs text-red-400 hover:text-red-300 hover:bg-red-950"
                    onClick={async () => {
                      if (!confirm(`Remove VPS ${vps.name}?`)) return;
                      await api.deleteVPS(vps.id);
                      load();
                    }}
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          ))}
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
    </div>
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
    <Modal title={vps ? "Edit VPS" : "Add VPS"} onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name">
          <input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} required autoFocus />
        </Field>
        
        <div className="flex gap-4">
          <div className="flex-[3]">
            <Field label="IP Address / Hostname">
              <input className="input w-full font-mono" value={ip} onChange={(e) => setIp(e.target.value)} required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="SSH Port">
              <input className="input w-full font-mono" type="number" min="1" max="65535" value={port} onChange={(e) => setPort(e.target.value)} required />
            </Field>
          </div>
        </div>

        <Field label="SSH Username">
          <input className="input w-full font-mono" value={user} onChange={(e) => setUser(e.target.value)} required />
        </Field>

        <Field label="SSH Key" hint="The private key used to authenticate.">
          <select 
            className="input w-full" 
            value={sshKeyId} 
            onChange={(e) => setSshKeyId(e.target.value)}
            required
          >
            <option value="" disabled>-- select a key --</option>
            {keys.map((k) => (
              <option key={k.id} value={k.id.toString()}>{k.name} ({k.type})</option>
            ))}
          </select>
        </Field>

        {err && <p className="mb-4 text-sm text-red-400">{err}</p>}
        
        <div className="flex justify-end gap-2 mt-6">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn-primary" disabled={busy || !name || !ip || !sshKeyId}>
            {busy ? "..." : "Save"}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function TerminalModal({ vps, onClose }: { vps: VPS; onClose: () => void }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  
  const [status, setStatus] = useState("Connecting...");

  useEffect(() => {
    if (!containerRef.current) return;
    
    const term = new Terminal({
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
      term.write("\r\n\x1b[31m[Connection Closed]\x1b[m\r\n");
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
          <span className="font-semibold">{vps.user}@{vps.name}</span>
          <span className={`text-xs px-2 py-0.5 rounded-full ${
            status === "Connected" ? "bg-green-500/20 text-green-400" :
            status === "Connecting..." ? "bg-yellow-500/20 text-yellow-400" :
            "bg-red-500/20 text-red-400"
          }`}>
            {status}
          </span>
        </div>
        <button 
          className="btn-ghost" 
          onClick={onClose}
        >
          Close Terminal
        </button>
      </div>
      <div className="flex-1 w-full relative">
        <div ref={containerRef} className="absolute inset-0 p-2" />
      </div>
    </div>
  );
}
