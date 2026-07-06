import { useEffect, useMemo, useRef, useState } from "react";
import { motion } from "framer-motion";
import { Search } from "lucide-react";
import { useTranslation } from "../i18n";
import { Area, SETTINGS_AREA } from "./areas";

export function CommandPalette({
  open,
  onClose,
  areas,
  onNavigate,
}: {
  open: boolean;
  onClose: () => void;
  areas: Area[];
  onNavigate: (path: string) => void;
}) {
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const { t } = useTranslation();

  // Flatten every nav item (areas + settings) into a flat, searchable list.
  // Labels are translated so search matches the language the user sees.
  const commands = useMemo(
    () =>
      [...areas, SETTINGS_AREA].flatMap((a) =>
        a.groups.flatMap((g) =>
          g.items.map((i) => ({
            id: i.path,
            label: t(`nav.${i.id}`, i.label),
            area: t(`areas.${a.id}.title`, a.title),
            group: t(`groups.${g.label}`, g.label),
            path: i.path,
            Icon: i.Icon,
            iconStr: i.iconStr,
          })),
        ),
      ),
    [areas, t],
  );

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return commands;
    return commands.filter(
      (c) =>
        c.label.toLowerCase().includes(needle) ||
        c.area.toLowerCase().includes(needle) ||
        c.group.toLowerCase().includes(needle) ||
        c.path.toLowerCase().includes(needle),
    );
  }, [q, commands]);

  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
      const t = setTimeout(() => inputRef.current?.focus(), 20);
      return () => clearTimeout(t);
    }
  }, [open]);
  useEffect(() => { setSel(0); }, [q]);

  if (!open) return null;

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(s + 1, filtered.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); const c = filtered[sel]; if (c) onNavigate(c.path); }
    else if (e.key === "Escape") { e.preventDefault(); onClose(); }
  };

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-black/50 px-4 pt-[12vh] backdrop-blur-sm"
      onClick={onClose}
    >
      <motion.div
        initial={{ opacity: 0, scale: 0.98, y: -8 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.14 }}
        className="glass-strong w-full max-w-xl overflow-hidden rounded-2xl shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-white/[0.08] px-4">
          <Search className="h-4 w-4 shrink-0 text-white/40" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder={t("command.placeholder")}
            className="w-full bg-transparent py-3.5 text-sm text-white placeholder:text-white/35 focus:outline-none"
          />
          <kbd className="shrink-0 rounded bg-white/[0.06] px-1.5 py-0.5 text-[10px] font-medium text-white/40">esc</kbd>
        </div>
        <div className="max-h-[50vh] overflow-y-auto p-1.5">
          {filtered.length === 0 ? (
            <div className="px-3 py-8 text-center text-sm text-white/40">{t("command.empty", { q })}</div>
          ) : (
            filtered.map((c, i) => (
              <button
                key={c.id}
                onMouseEnter={() => setSel(i)}
                onClick={() => onNavigate(c.path)}
                className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors ${
                  i === sel ? "bg-white/[0.08]" : "hover:bg-white/[0.04]"
                }`}
              >
                {c.iconStr ? (
                  <span className="w-4 text-center text-sm">{c.iconStr}</span>
                ) : (
                  <c.Icon className="h-4 w-4 shrink-0 text-white/60" strokeWidth={1.75} />
                )}
                <span className="flex-1 truncate text-sm text-white">{c.label}</span>
                <span className="shrink-0 text-[11px] text-white/35">{c.area} · {c.group}</span>
              </button>
            ))
          )}
        </div>
      </motion.div>
    </div>
  );
}
