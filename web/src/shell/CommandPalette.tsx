import { useEffect, useMemo, useRef, useState } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { Search } from "lucide-react";
import { useTranslation } from "../i18n";
import { Area } from "./areas";
import { Action } from "../api";
import { CommandPaletteItem, mergeCommandItems } from "./globalActions";
import {
  translateAreaTitle,
  translateGroupLabel,
  translateNavItemLabel,
} from "./navI18n";

export function CommandPalette({
  open,
  onClose,
  areas,
  settingsArea,
  onNavigate,
  actions = [],
}: {
  open: boolean;
  onClose: () => void;
  areas: Area[];
  // Admin-filtered merged Settings area with plugin-contributed settings pages.
  settingsArea: Area;
  onNavigate: (path: string) => void;
  actions?: Action[];
}) {
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const { t } = useTranslation();

  // Flatten nav items (areas + settings) with translated labels for search.
  const navItems = useMemo<CommandPaletteItem[]>(
    () =>
      [...areas, settingsArea].flatMap((a) =>
        a.groups.flatMap((g) =>
          g.items.flatMap((i) => {
            const areaTitle = translateAreaTitle(t, a.id, a.title);
            const groupLabel = translateGroupLabel(t, g.label);
            return [
              {
                id: i.path,
                label: translateNavItemLabel(t, i.id, i.label),
                area: areaTitle,
                group: groupLabel,
                path: i.path,
                Icon: i.Icon,
                iconStr: i.iconStr,
                isAction: false,
              },
            ];
          }),
        ),
      ),
    [areas, settingsArea, t],
  );

  const commands = useMemo(
    () =>
      mergeCommandItems(
        actions.map((a) => ({ ...a, label: translateNavItemLabel(t, a.id, a.label) })),
        navItems,
      ),
    [actions, navItems, t],
  );

  const areaTitles = useMemo(
    () => [...areas, settingsArea].map((a) => translateAreaTitle(t, a.id, a.title)),
    [areas, settingsArea, t],
  );

  const filtered = useMemo(() => {
    const nonDocCommands = commands.filter(
      (c) => !c.path.startsWith("/help") && !c.path.startsWith("/admin/help"),
    );
    const needle = q.trim().toLowerCase();
    if (!needle) {
      return nonDocCommands;
    }
    return nonDocCommands.filter(
      (c) =>
        c.label.toLowerCase().includes(needle) ||
        (c.isAction ? c.category?.toLowerCase().includes(needle) : false) ||
        (!c.isAction && c.area?.toLowerCase().includes(needle)) ||
        (!c.isAction && c.group?.toLowerCase().includes(needle)) ||
        c.path.toLowerCase().includes(needle),
    );
  }, [q, commands]);

  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
    }
  }, [open]);
  useEffect(() => { setSel(0); }, [q]);

  // Arrow/Enter drive result list navigation.
  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setSel((s) => Math.min(s + 1, filtered.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setSel((s) => Math.max(s - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); const c = filtered[sel]; if (c) onNavigate(c.path); }
  };

  return (
    <BaseDialog.Root open={open} onOpenChange={(next) => { if (!next) onClose(); }}>
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-[100] bg-black/50 backdrop-blur-sm modal-overlay" />
        <BaseDialog.Popup
          initialFocus={inputRef}
          aria-label={t("command.placeholder")}
          className="glass-strong fixed left-1/2 top-[12vh] z-[100] w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 overflow-hidden rounded-lg modal-card outline-none"
        >
        <div className="flex items-center gap-3 border-b border-foreground/[0.08] dark:border-white/[0.08] focus-within:border-primary/50 bg-foreground/[0.015] dark:bg-white/[0.015] px-4 py-1 transition-colors">
          <Search className="h-4 w-4 shrink-0 text-primary" />
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={onKey}
            placeholder={t("command.placeholder")}
            className="w-full bg-transparent py-3 text-sm font-medium text-foreground placeholder:text-muted-foreground/60 outline-none border-none ring-0 focus:outline-none focus:border-none focus:ring-0 focus:shadow-none focus-visible:outline-none focus-visible:ring-0"
          />
          <kbd className="shrink-0 rounded-md border border-foreground/10 dark:border-white/10 bg-muted/60 px-1.5 py-0.5 text-[10px] font-mono font-medium text-muted-foreground">esc</kbd>
        </div>
        <div className="max-h-[50vh] overflow-y-auto p-2 scrollbar-thin">
          {filtered.length === 0 ? (
            <div className="px-3 py-8 text-center">
              <p className="text-sm text-muted-foreground">
                {t("command.emptyTitle")} <span className="font-mono">{`“${q}”`}</span>
              </p>
              <p className="mx-auto mt-1.5 max-w-sm text-xs leading-relaxed text-muted-foreground/70">
                {t("command.emptyHint", { areas: areaTitles.join(", ") })}
              </p>
            </div>
          ) : (
            filtered.map((c, i) => (
              <button
                key={c.id}
                onMouseEnter={() => setSel(i)}
                onClick={() => onNavigate(c.path)}
                className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-all ${
                  i === sel
                    ? "bg-primary/10 text-primary dark:bg-primary/20 font-medium"
                    : "hover:bg-surface-hover/80 text-foreground/90"
                }`}
              >
                {c.iconStr ? (
                  <span className="w-4 text-center text-sm">{c.iconStr}</span>
                ) : c.Icon ? (
                  <c.Icon className={`h-4 w-4 shrink-0 transition-colors ${i === sel ? "text-primary" : "text-muted-foreground"}`} strokeWidth={1.75} />
                ) : null}
                <span className="flex-1 truncate text-sm">{c.label}</span>
                <span className="shrink-0 rounded-md border border-foreground/5 dark:border-white/5 bg-muted/40 dark:bg-white/5 px-2 py-0.5 text-[10px] font-mono text-muted-foreground">
                  {c.isAction ? t("command.create", "Create") : `${c.area} · ${c.group}`}
                </span>
              </button>
            ))
          )}
        </div>
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
