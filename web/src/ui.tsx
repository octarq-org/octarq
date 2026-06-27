import { ReactNode, useEffect, useState } from "react";
import { twMerge } from "tailwind-merge";
import { motion } from "framer-motion";
import { HostEntry } from "./api";

// ─── GlassCard ──────────────────────────────────────────────────────────────

export function GlassCard({
  className,
  children,
  strong,
}: {
  className?: string;
  children: ReactNode;
  strong?: boolean;
}) {
  return (
    <div className={twMerge(strong ? "glass-strong" : "glass", "rounded-2xl", className)}>
      {children}
    </div>
  );
}

// ─── Badge ───────────────────────────────────────────────────────────────────

type BadgeTone = "indigo" | "violet" | "green" | "amber" | "red" | "neutral" | "cyan";

const BADGE_TONE: Record<BadgeTone, string> = {
  indigo:  "bg-indigo-500/15 text-indigo-300 ring-indigo-400/20",
  violet:  "bg-violet-500/15 text-violet-300 ring-violet-400/20",
  green:   "bg-emerald-500/15 text-emerald-300 ring-emerald-400/20",
  amber:   "bg-amber-500/15  text-amber-300  ring-amber-400/20",
  red:     "bg-rose-500/15   text-rose-300   ring-rose-400/20",
  cyan:    "bg-cyan-500/15   text-cyan-300   ring-cyan-400/20",
  neutral: "bg-white/[0.08]  text-white/70   ring-white/10",
};

export function Badge({
  children,
  tone = "neutral",
  className,
}: {
  children: ReactNode;
  tone?: BadgeTone;
  className?: string;
}) {
  return (
    <span
      className={twMerge(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset",
        BADGE_TONE[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

// ─── Button ──────────────────────────────────────────────────────────────────

type ButtonVariant = "primary" | "ghost" | "outline" | "subtle" | "danger";

const BTN_VARIANT: Record<ButtonVariant, string> = {
  primary: "bg-indigo-500 text-white hover:bg-indigo-400 shadow-[0_8px_30px_-8px_rgba(99,102,241,0.6)]",
  ghost:   "text-white/65 hover:text-white hover:bg-white/5",
  outline: "border border-white/10 text-white/80 hover:bg-white/5 hover:border-white/20",
  subtle:  "bg-white/5 text-white/80 hover:bg-white/10",
  danger:  "text-rose-300/90 hover:bg-rose-500/10 hover:text-rose-300",
};

export function Button({
  children,
  variant = "primary",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant }) {
  return (
    <button
      className={twMerge(
        "inline-flex items-center justify-center gap-2 rounded-xl px-3.5 py-2 text-sm font-medium transition-colors duration-150 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60 disabled:cursor-not-allowed disabled:opacity-50",
        BTN_VARIANT[variant],
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

// ─── ProPill ─────────────────────────────────────────────────────────────────

export function ProPill({ className }: { className?: string }) {
  return (
    <span
      className={twMerge(
        "inline-flex items-center gap-1 rounded-full bg-gradient-to-r from-indigo-500/25 to-violet-500/25 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-violet-200 ring-1 ring-inset ring-violet-400/30",
        className,
      )}
    >
      Pro
    </span>
  );
}

// ─── StatCard ─────────────────────────────────────────────────────────────────

export function StatCard({
  label,
  value,
  delta,
  positive = true,
  icon,
  index = 0,
  onClick,
}: {
  label: string;
  value: string | number;
  delta?: string;
  positive?: boolean;
  icon?: ReactNode;
  index?: number;
  onClick?: () => void;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay: index * 0.05 }}
      onClick={onClick}
      className={twMerge(
        "glass rounded-2xl p-4 text-left transition-all duration-150",
        onClick ? "cursor-pointer hover:bg-white/[0.06] active:scale-[0.98]" : ""
      )}
    >
      <div className="mb-2 flex items-center justify-between">
        <span className="text-[12px] font-medium text-white/45">{label}</span>
        {icon && <span className="text-white/40">{icon}</span>}
      </div>
      <div className="flex items-end gap-2">
        <span className="font-display text-2xl font-bold tracking-tight text-white">
          {value}
        </span>
        {delta && (
          <span
            className={`mb-1 text-[12px] font-medium ${positive ? "text-emerald-400" : "text-rose-400"}`}
          >
            {delta}
          </span>
        )}
      </div>
    </motion.div>
  );
}

// ─── PageHeader ───────────────────────────────────────────────────────────────

export function PageHeader({
  title,
  description,
  action,
}: {
  title: ReactNode;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="font-display text-2xl font-bold tracking-tight text-white">{title}</h1>
        {description && <p className="mt-1 text-sm text-white/50">{description}</p>}
      </div>
      {action}
    </div>
  );
}

// ─── ScreenWrap ───────────────────────────────────────────────────────────────

export function ScreenWrap({ children }: { children: ReactNode }) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25, ease: "easeOut" }}
    >
      {children}
    </motion.div>
  );
}

// ─── Modal ───────────────────────────────────────────────────────────────────

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
        className={`glass-strong w-full rounded-2xl p-5 modal-card ${wide ? "max-w-3xl" : "max-w-md"}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="font-display text-lg font-semibold text-white">{title}</h2>
          <button className="btn-ghost rounded-xl px-2 py-1 text-white/50 hover:text-white" onClick={onClose}>
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

// ─── Field ───────────────────────────────────────────────────────────────────

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
      {hint && <p className="mt-1 text-xs text-white/40">{hint}</p>}
    </div>
  );
}

// ─── Empty ───────────────────────────────────────────────────────────────────

export function Empty({ children }: { children: ReactNode }) {
  return (
    <div className="glass flex flex-col items-center justify-center gap-2 rounded-2xl py-16 text-white/45">
      {children}
    </div>
  );
}

// ─── Toggle ──────────────────────────────────────────────────────────────────

export function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!on)}
      className={`relative h-5 w-9 rounded-full transition-all duration-300 ${on ? "bg-indigo-500" : "bg-white/15"}`}
    >
      <span
        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition-all duration-300 shadow-sm ${
          on ? "left-4 scale-110" : "left-0.5 scale-90 opacity-70"
        }`}
      />
    </button>
  );
}

// ─── HostList ─────────────────────────────────────────────────────────────────

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
  baseDomain?: string;
  emptyText?: string;
}) {
  const [draft, setDraft] = useState("");

  function resolve(raw: string): string {
    const v = raw.trim().toLowerCase();
    if (!v) return v;
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
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-2.5">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {hosts.length === 0 ? (
          <span className="text-xs text-white/40">{emptyText}</span>
        ) : (
          hosts.map((h) => (
            <span
              key={h.host}
              className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-sm border transition-colors ${
                h.enabled
                  ? "bg-indigo-500/15 text-indigo-200 border-indigo-500/25"
                  : "bg-white/5 text-white/35 border-white/10 line-through"
              }`}
            >
              <button
                type="button"
                className={`cursor-pointer hover:text-white text-xs ${h.enabled ? "text-indigo-400" : "text-white/35"}`}
                title={h.enabled ? "Disable host" : "Enable host"}
                onClick={() =>
                  onChange(hosts.map((x) => (x.host === h.host ? { ...x, enabled: !x.enabled } : x)))
                }
              >
                {h.enabled ? "●" : "○"}
              </button>
              <span className="select-none">{h.host}</span>
              <button
                type="button"
                className="text-white/30 hover:text-rose-400 ml-0.5"
                title="remove"
                onClick={() => onChange(hosts.filter((x) => x.host !== h.host))}
              >
                ✕
              </button>
            </span>
          ))
        )}
      </div>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <input
            className="input w-full"
            value={draft}
            placeholder={placeholder ?? "type a host and press Enter"}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") { e.preventDefault(); add(draft); }
            }}
          />
          {baseDomain && draft && !draft.includes(".") && (
            <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-xs text-white/35">
              → {draft.trim().toLowerCase()}.{baseDomain}
            </span>
          )}
        </div>
        <button className="btn-primary shrink-0" type="button" disabled={!draft.trim()} onClick={() => add(draft)}>
          + Add
        </button>
      </div>
      {freshSuggestions.length > 0 && (
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-white/40">Quick add:</span>
          {freshSuggestions.map((s) => (
            <button
              key={s}
              type="button"
              className="rounded-lg border border-indigo-500/35 px-2 py-0.5 text-xs text-indigo-300 transition hover:bg-indigo-500/10"
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

// ─── Guide ───────────────────────────────────────────────────────────────────

export function Guide({
  title,
  children,
  open = false,
}: {
  title: string;
  children: ReactNode;
  open?: boolean;
}) {
  const [show, setShow] = useState(open);
  return (
    <div className="glass mb-3 overflow-hidden rounded-2xl text-sm">
      <button
        className="flex w-full items-center justify-between px-3 py-2 text-left text-white/70 hover:bg-white/5"
        onClick={() => setShow((s) => !s)}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-white/35">{show ? "▾" : "▸"}</span>
      </button>
      {show && (
        <div className="space-y-2 border-t border-white/[0.06] px-3 py-3 text-white/55 animate-expand">
          {children}
        </div>
      )}
    </div>
  );
}

// ─── Code ────────────────────────────────────────────────────────────────────

export function Code({ children }: { children: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <code
      className="cursor-pointer break-all rounded-lg bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px] text-indigo-200 hover:bg-white/10"
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

// ─── AreaChart ────────────────────────────────────────────────────────────────

export function AreaChart({
  series,
  height = 120,
}: {
  series: { key: string; count: number }[];
  height?: number;
}) {
  if (!series || !series.length)
    return <div className="grid h-28 place-items-center text-sm text-white/35">No data yet</div>;

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

// ─── BarList ──────────────────────────────────────────────────────────────────

export function BarList({
  rows,
  empty = "—",
}: {
  rows: { key: string; count: number }[] | null;
  empty?: string;
}) {
  if (!rows || rows.length === 0) return <p className="text-sm text-white/35">{empty}</p>;
  const max = Math.max(...rows.map((r) => r.count), 1);
  return (
    <div className="space-y-1.5">
      {rows.map((r) => (
        <div key={r.key} className="flex items-center gap-2 text-sm">
          <span className="w-24 truncate text-white/70">{r.key || "(direct)"}</span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-white/8">
            <div
              className="h-full rounded-full bg-indigo-500/60"
              style={{ width: `${(r.count / max) * 100}%` }}
            />
          </div>
          <span className="w-8 text-right text-white/40">{r.count}</span>
        </div>
      ))}
    </div>
  );
}

// ─── timeAgo ──────────────────────────────────────────────────────────────────

export function timeAgo(iso: string): string {
  const d = new Date(iso).getTime();
  const s = Math.floor((Date.now() - d) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}
