// Small shared UI primitives.
import { ReactNode, useEffect, useState } from "react";

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
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 pt-16"
      onClick={onClose}
    >
      <div
        className={`card w-full ${wide ? "max-w-3xl" : "max-w-md"} p-5 shadow-2xl`}
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
      className={`relative h-5 w-9 rounded-full transition ${on ? "bg-indigo-500" : "bg-zinc-700"}`}
    >
      <span
        className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition ${
          on ? "left-4" : "left-0.5"
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
}: {
  hosts: string[];
  onChange: (hosts: string[]) => void;
  suggestions?: string[];
  placeholder?: string;
}) {
  const [draft, setDraft] = useState("");
  function add(h: string) {
    const v = h.trim().toLowerCase();
    if (v && !hosts.includes(v)) onChange([...hosts, v]);
    setDraft("");
  }
  return (
    <div>
      <div className="mb-1.5 flex flex-wrap gap-1.5">
        {hosts.length === 0 && <span className="text-xs text-zinc-500">none — defaults to the apex domain</span>}
        {hosts.map((h) => (
          <span key={h} className="badge bg-indigo-500/15 text-indigo-200">
            {h}
            <button className="ml-1 text-zinc-400 hover:text-red-400" onClick={() => onChange(hosts.filter((x) => x !== h))}>
              ✕
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-2">
        <input
          className="input"
          value={draft}
          placeholder={placeholder}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add(draft);
            }
          }}
        />
        <button className="btn-ghost shrink-0" type="button" onClick={() => add(draft)}>
          Add
        </button>
      </div>
      {suggestions.filter((s) => !hosts.includes(s)).length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {suggestions
            .filter((s) => !hosts.includes(s))
            .map((s) => (
              <button key={s} type="button" className="badge cursor-pointer hover:bg-zinc-700" onClick={() => add(s)}>
                + {s}
              </button>
            ))}
        </div>
      )}
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
