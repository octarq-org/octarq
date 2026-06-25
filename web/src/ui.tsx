// Small shared UI primitives.
import { ReactNode, useEffect, useState } from "react";
import { HostEntry } from "./api";


export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto p-4 pt-16 modal-overlay"
      onClick={onClose}
    >
      <div
        className={`card w-full ${wide ? "max-w-3xl" : "max-w-md"} p-5 modal-card`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold">{title}</h2>
          <button className="btn-ghost px-2" onClick={onClose}>
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <div className="mb-3">
      <label className="label">{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-zinc-500">{hint}</p>}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="card flex flex-col items-center justify-center gap-2 py-16 text-zinc-500">
      {children}
    </div>
  );
}

export function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!on)}
      className={`relative h-5 w-9 rounded-full transition-all duration-300 ease-in-out ${on ? "bg-indigo-500 shadow-inner shadow-indigo-900/50" : "bg-zinc-700"}`}
    >
      <span
        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all duration-300 ease-in-out shadow-sm ${
          on ? "left-4 scale-110" : "left-0.5 scale-90 opacity-70"
        }`}
      />
    </button>
  );
}

// HostList edits a list of hostnames (chips) with an add input and one-click
// suggestion chips. Used for a domain's short-link and mail hosts.
export function HostList({
  hosts,
  onChange,
  suggestions = [],
  placeholder,
  baseDomain,
  emptyText = "No hosts added yet.",
}: {
  hosts: HostEntry[];
  onChange: (hosts: HostEntry[]) => void;
  suggestions?: string[];
  placeholder?: string;
  /** When provided, bare labels without a dot are auto-expanded: "go" → "go.example.com" */
  baseDomain?: string;
  /** Text shown when the host list is empty. */
  emptyText?: string;
}) {
  const [draft, setDraft] = useState("");
  function resolve(raw: string): string {
    const v = raw.trim().toLowerCase();
    if (!v) return v;
    // Auto-append base domain when the user types a bare label (no dot)
    if (baseDomain && !v.includes(".")) return `${v}.${baseDomain}`;
    return v;
  }
  function add(h: string) {
    const v = resolve(h);
    if (v && !hosts.some((x) => x.host === v)) {
      onChange([...hosts, { host: v, enabled: true }]);
    }
    setDraft("");
  }
  const freshSuggestions = suggestions.filter((s) => !hosts.some((x) => x.host === s));
  return (
    <div className="rounded-lg border border-zinc-700 bg-zinc-900/40 p-2.5">
      {/* selected hosts */}
      <div className="mb-2 flex flex-wrap gap-1.5">
        {hosts.length === 0 ? (
          <span className="text-xs text-zinc-500">{emptyText}</span>
        ) : (
          hosts.map((h) => (
            <span
              key={h.host}
              className={`inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-sm border transition-colors ${
                h.enabled
                  ? "bg-indigo-500/20 text-indigo-100 border-indigo-500/30"
                  : "bg-zinc-800 text-zinc-500 border-zinc-700 line-through"
              }`}
            >
              <button
                type="button"
                className={`cursor-pointer hover:text-white text-xs ${h.enabled ? "text-indigo-400" : "text-zinc-500"}`}
                title={h.enabled ? "Disable host" : "Enable host"}
                onClick={() =>
                  onChange(
                    hosts.map((x) =>
                      x.host === h.host ? { ...x, enabled: !x.enabled } : x
                    )
                  )
                }
              >
                {h.enabled ? "●" : "○"}
              </button>
              <span className="select-none">{h.host}</span>
              <button
                type="button"
                className="text-zinc-400 hover:text-red-400 ml-0.5"
                title="remove"
                onClick={() => onChange(hosts.filter((x) => x.host !== h.host))}
              >
                ✕
              </button>
            </span>
          ))
        )}
      </div>

      {/* add row */}
      <div className="flex gap-2">
        <div className="relative flex-1">
          <input
            className="input w-full"
            value={draft}
            placeholder={placeholder ?? "type a host and press Enter"}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                add(draft);
              }
            }}
          />
          {baseDomain && draft && !draft.includes(".") && (
            <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-zinc-500">
              → {draft.trim().toLowerCase()}.{baseDomain}
            </span>
          )}
        </div>
        <button className="btn-primary shrink-0" type="button" disabled={!draft.trim()} onClick={() => add(draft)}>
          + Add
        </button>
      </div>

      {/* quick-add suggestions */}
      {freshSuggestions.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-zinc-500">Quick add:</span>
          {freshSuggestions.map((s) => (
            <button
              key={s}
              type="button"
              className="rounded-md border border-indigo-500/40 px-2 py-0.5 text-xs text-indigo-200 transition hover:bg-indigo-500/15"
              onClick={() => add(s)}
            >
              + {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Guide is a collapsible help/instructions panel.
export function Guide({ title, children, open = false }: { title: string; children: ReactNode; open?: boolean }) {
  const [show, setShow] = useState(open);
  return (
    <div className="card mb-3 overflow-hidden text-sm">
      <button
        className="flex w-full items-center justify-between px-3 py-2 text-left text-zinc-300 hover:bg-zinc-800/50"
        onClick={() => setShow((s) => !s)}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-zinc-500">{show ? "▾" : "▸"}</span>
      </button>
      {show && <div className="space-y-2 border-t border-zinc-800 px-3 py-3 text-zinc-400 animate-expand">{children}</div>}
    </div>
  );
}

// Code renders an inline, click-to-copy code snippet.
export function Code({ children }: { children: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <code
      className="cursor-pointer break-all rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-[12px] text-indigo-200 hover:bg-zinc-700"
      title="click to copy"
      onClick={() => {
        navigator.clipboard.writeText(children);
        setCopied(true);
        setTimeout(() => setCopied(false), 1000);
      }}
    >
      {copied ? "copied ✓" : children}
    </code>
  );
}

// AreaChart is a dependency-free SVG area/line chart for a daily series.
export function AreaChart({ series, height = 120 }: { series: { key: string; count: number }[]; height?: number }) {
  if (!series.length) return <div className="grid h-28 place-items-center text-sm text-zinc-600">No data yet</div>;
  const w = 600;
  const h = height;
  const pad = 6;
  const max = Math.max(...series.map((s) => s.count), 1);
  const n = series.length;
  const x = (i: number) => (n === 1 ? w / 2 : pad + (i * (w - 2 * pad)) / (n - 1));
  const y = (v: number) => h - pad - (v / max) * (h - 2 * pad);
  const pts = series.map((s, i) => `${x(i)},${y(s.count)}`);
  const line = `M ${pts.join(" L ")}`;
  const area = `${line} L ${x(n - 1)},${h - pad} L ${x(0)},${h - pad} Z`;
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="w-full" preserveAspectRatio="none" style={{ height }}>
      <defs>
        <linearGradient id="led-area" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="rgb(99 102 241)" stopOpacity="0.5" />
          <stop offset="100%" stopColor="rgb(99 102 241)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill="url(#led-area)" />
      <path d={line} fill="none" stroke="rgb(129 140 248)" strokeWidth="2" vectorEffect="non-scaling-stroke" />
      {series.map((s, i) => (
        <circle key={i} cx={x(i)} cy={y(s.count)} r="2.5" fill="rgb(129 140 248)">
          <title>{`${s.key}: ${s.count}`}</title>
        </circle>
      ))}
    </svg>
  );
}

// BarList renders a labeled horizontal bar list (top countries/devices, etc.).
export function BarList({ rows, empty = "—" }: { rows: { key: string; count: number }[] | null; empty?: string }) {
  if (!rows || rows.length === 0) return <p className="text-sm text-zinc-600">{empty}</p>;
  const max = Math.max(...rows.map((r) => r.count), 1);
  return (
    <div className="space-y-1.5">
      {rows.map((r) => (
        <div key={r.key} className="flex items-center gap-2 text-sm">
          <span className="w-24 truncate text-zinc-300">{r.key || "(direct)"}</span>
          <div className="h-2 flex-1 overflow-hidden rounded bg-zinc-800">
            <div className="h-full rounded bg-indigo-500/70" style={{ width: `${(r.count / max) * 100}%` }} />
          </div>
          <span className="w-8 text-right text-zinc-500">{r.count}</span>
        </div>
      ))}
    </div>
  );
}

export function timeAgo(iso: string): string {
  const d = new Date(iso).getTime();
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}
