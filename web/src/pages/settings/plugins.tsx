import { useEffect, useState } from "react";
import { api, ApiError, PluginInfo } from "../../api";
import { Toggle, PageHeader, GlassCard, Badge, Alert, Tooltip, confirmDialog } from "../../ui";
import { ShieldAlert, Puzzle, Search, Tag, Info } from "lucide-react";
import { useTranslation } from "../../i18n";
import { menuIcon } from "../../shell/areas";
import { useSettingsData } from "./shared";

// Category display metadata & tones
const CATEGORIES: Record<string, { labelKey: string; tone: "indigo" | "amber" | "green" | "violet" | "cyan" | "red" | "neutral" }> = {
  marketing: { labelKey: "settings.pluginCategory.marketing", tone: "indigo" },
  messaging: { labelKey: "settings.pluginCategory.messaging", tone: "cyan" },
  infrastructure: { labelKey: "settings.pluginCategory.infrastructure", tone: "violet" },
  security: { labelKey: "settings.pluginCategory.security", tone: "amber" },
  commerce: { labelKey: "settings.pluginCategory.commerce", tone: "green" },
  ai: { labelKey: "settings.pluginCategory.ai", tone: "indigo" },
  utilities: { labelKey: "settings.pluginCategory.utilities", tone: "neutral" },
};

// Helper component to render a plugin's icon or custom logo.
function PluginIcon({ iconStr, firstMenuIcon }: { iconStr?: string; firstMenuIcon?: string }) {
  const { t } = useTranslation();
  const target = iconStr || firstMenuIcon;
  if (!target) {
    return <Puzzle className="h-5 w-5 text-accent-fg" />;
  }

  // Check if target is an image URL or data URI
  if (target.startsWith("http://") || target.startsWith("https://") || target.startsWith("data:") || target.startsWith("/")) {
    return <img src={target} alt={t("settings.pluginLogoAlt")} className="h-6 w-6 object-contain rounded-lg" />;
  }

  // Resolve via single menuIcon map in shell/areas
  const IconComp = menuIcon(target);
  if (IconComp) {
    return <IconComp className="h-5 w-5 text-accent-fg" />;
  }

  // Render text / emoji
  return <span className="text-lg leading-none select-none">{target}</span>;
}

// lockedBy returns the enabled features that depend on this one, i.e. the
// reason its toggle can't be turned off. Empty when the feature is free to
// disable — including when it is already off, since nothing can depend on a
// feature that isn't running.
function lockedBy(p: PluginInfo): string[] {
  return p.enabled ? (p.requiredBy ?? []) : [];
}

export function PluginsSettings() {
  const { s: settings } = useSettingsData();
  const { t } = useTranslation();
  const [plugins, setPlugins] = useState<PluginInfo[] | null>(null);
  const [err, setErr] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "enabled" | "disabled">("all");
  const [categoryFilter, setCategoryFilter] = useState<string>("all");

  function load() {
    api.plugins().then(setPlugins).catch((e: ApiError) => setErr(e.message || t("settings.failedLoadPlugins")));
  }
  useEffect(load, []);

  async function toggle(key: string, enabled: boolean) {
    setErr("");
    const target = plugins?.find(p => p.key === key);
    if (enabled && target && target.requires && target.requires.length > 0) {
      const disabledDeps = target.requires.filter(d => {
        const dep = plugins?.find(p => p.key === d);
        return dep && !dep.enabled;
      });
      if (disabledDeps.length > 0) {
        const ok = await confirmDialog({
          title: t("settings.pluginsTitle"),
          message: t("settings.enableDepsConfirm", { deps: disabledDeps.join(", ") }),
          confirmLabel: t("settings.badgeOn"),
          danger: false,
        });
        if (!ok) return;
      }
    }
    
    // optimistic update; revert on failure
    setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled } : p)) ?? prev);
    try {
      await api.updatePlugin(key, enabled);
      // Reload plugins completely to get new status of dependencies
      load();
      window.dispatchEvent(new CustomEvent("octarq:plugins-changed"));
    } catch (e) {
      setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled: !enabled } : p)) ?? prev);
      if (e instanceof ApiError && e.status === 409) {
        let msg = e.message;
        if (e.body && e.body.dependents) {
           msg = t("settings.pluginInUse", { plugin: key, dependents: e.body.dependents.join(", ") });
        }
        setErr(msg);
      } else {
        setErr(e instanceof ApiError ? e.message : t("settings.failedUpdatePlugin"));
      }
    }
  }

  // Compute available categories from plugins list
  const availableCategories = Array.from(
    new Set((plugins ?? []).map((p) => p.category || "utilities"))
  );

  const filteredPlugins = (plugins ?? []).filter((p) => {
    const title = p.title.toLowerCase();
    const key = p.key.toLowerCase();
    const desc = (p.description || "").toLowerCase();
    const cat = (p.category || "utilities").toLowerCase();
    const tags = (p.tags || []).join(" ").toLowerCase();
    const query = search.toLowerCase().trim();

    const matchesSearch = !query || title.includes(query) || key.includes(query) || desc.includes(query) || tags.includes(query);
    const matchesStatus =
      statusFilter === "all" ||
      (statusFilter === "enabled" && p.enabled) ||
      (statusFilter === "disabled" && !p.enabled);
    const matchesCategory = categoryFilter === "all" || cat === categoryFilter;

    return matchesSearch && matchesStatus && matchesCategory;
  });

  const enabledCount = (plugins ?? []).filter((p) => p.enabled).length;

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.pluginsTitle")} description={t("settings.pluginsDescription")} />

      {/* The card grid reads like an app store, so say plainly what a toggle
          does: it scopes to this workspace and installs nothing. The wording
          stays in workspace terms — deployment-level concepts belong in the
          console, behind the link only instance admins see. */}
      <Alert variant="info" icon={<Info className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
        <span>{t("settings.pluginsScopeNote")}</span>
        {settings?.isInstanceAdmin && (
          <>
            {" "}
            <a href="/instance/plugins" className="underline underline-offset-2 hover:text-accent-fg">
              {t("settings.pluginsInstanceLink")}
            </a>
          </>
        )}
      </Alert>

      {err && (
        <Alert variant="danger" icon={<ShieldAlert className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
          {err}
        </Alert>
      )}

      {plugins !== null && plugins.length > 0 && (
        <div className="space-y-3">
          {/* Top Category Tag Bar */}
          <div className="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
            <button
              onClick={() => setCategoryFilter("all")}
              className={`px-3 py-1.5 rounded-xl font-medium transition-colors shrink-0 ${
                categoryFilter === "all"
                  ? "bg-accent-fg text-white shadow-xs"
                  : "bg-foreground/[0.04] text-foreground/70 hover:bg-surface-hover hover:text-foreground"
              }`}
            >
              {t("settings.allCategoriesCount", { count: plugins.length })}
            </button>
            {availableCategories.map((c) => {
              const meta = CATEGORIES[c] || { labelKey: "settings.pluginCategory." + c, tone: "neutral" };
              const count = (plugins ?? []).filter((p) => (p.category || "utilities") === c).length;
              const isSelected = categoryFilter === c;
              return (
                <button
                  key={c}
                  onClick={() => setCategoryFilter(c)}
                  className={`px-3 py-1.5 rounded-xl font-medium transition-colors shrink-0 capitalize ${
                    isSelected
                      ? "bg-accent-fg text-white shadow-xs"
                      : "bg-foreground/[0.04] text-foreground/70 hover:bg-surface-hover hover:text-foreground"
                  }`}
                >
                  {t(meta.labelKey)} ({count})
                </button>
              );
            })}
          </div>

          {/* Search & Status Filter Control */}
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-foreground/[0.02] p-3 rounded-2xl border border-border/60">
            <div className="relative flex-1 max-w-md">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("common.search")}
                className="w-full pl-9 pr-3 py-1.5 text-xs rounded-xl bg-card border border-border focus:outline-none focus:ring-2 focus:ring-accent-fg/30 text-foreground placeholder:text-muted-foreground"
              />
            </div>

            <div className="flex items-center gap-1 bg-surface rounded-xl p-1 border border-border/40 text-xs shrink-0">
              <button
                onClick={() => setStatusFilter("all")}
                className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                  statusFilter === "all"
                    ? "bg-card text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t("settings.filterAllCount", { count: plugins.length })}
              </button>
              <button
                onClick={() => setStatusFilter("enabled")}
                className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                  statusFilter === "enabled"
                    ? "bg-card text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t("settings.filterActiveCount", { count: enabledCount })}
              </button>
              <button
                onClick={() => setStatusFilter("disabled")}
                className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                  statusFilter === "disabled"
                    ? "bg-card text-foreground shadow-xs"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                {t("settings.filterDisabledCount", { count: plugins.length - enabledCount })}
              </button>
            </div>
          </div>
        </div>
      )}

      {plugins === null ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">{t("settings.loadingPlugins")}</GlassCard>
      ) : plugins.length === 0 ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/55">{t("settings.noPlugins")}</GlassCard>
      ) : filteredPlugins.length === 0 ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">
          {t("settings.noMatchingPlugins")}
        </GlassCard>
      ) : (
        /* Card Grid Layout */
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredPlugins.map((p) => {
            const description = t("settings.pluginDesc." + p.key, p.description || "");
            const firstMenuIcon = p.menus?.[0]?.icon;
            const categoryMeta = CATEGORIES[p.category || "utilities"] || { labelKey: "settings.pluginCategory." + (p.category || "utilities"), tone: "neutral" };

            return (
              <GlassCard
                key={p.key}
                className={`p-5 flex flex-col justify-between transition-all duration-200 hover:border-accent-fg/40 ${
                  p.enabled ? "ring-1 ring-accent-fg/20 bg-card" : "opacity-80"
                }`}
              >
                <div>
                  {/* Card Top Header */}
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-foreground/[0.04] border border-border/60 shadow-xs">
                        <PluginIcon iconStr={p.icon} firstMenuIcon={firstMenuIcon} />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <h3 className="text-sm font-bold text-foreground truncate">{p.title}</h3>
                          {/* Named on the card, not just implied by a dead
                              switch: a disabled control with no label reads as
                              broken, and the tooltip explaining it is invisible
                              to touch and keyboard. */}
                          {p.core && (
                            <Badge tone="violet" className="shrink-0 text-[9px] uppercase tracking-wider font-semibold">
                              {t("settings.pluginCoreBadge")}
                            </Badge>
                          )}
                        </div>
                        <span className="text-[10px] font-mono text-muted-foreground block truncate">@{p.key}</span>
                      </div>
                    </div>

                    {/* Locked while something else depends on it. The server
                        enforces this with a 409 regardless — this only saves
                        the user a round trip to be told no. A dead switch is
                        only honest if it says why, so carry the reason as a
                        tooltip too: the card's warning line sits below the
                        fold on a narrow card, and hovering the thing that
                        didn't respond is where people look first. */}
                    <div className="flex flex-col items-end gap-1">
                      {p.core ? (
                        // Core outranks the dependency lock: it can't be turned
                        // off for any workspace, so say that rather than naming
                        // a dependent that isn't the real reason.
                        <Tooltip content={t("settings.pluginCoreHint")}>
                          <Toggle on onChange={() => {}} disabled aria-label={p.title} />
                        </Tooltip>
                      ) : lockedBy(p).length > 0 ? (
                        <Tooltip
                          content={t("settings.pluginInUse", { plugin: p.title, dependents: lockedBy(p).join(", ") })}
                        >
                          <Toggle on={p.enabled} onChange={() => {}} disabled aria-label={p.title} />
                        </Tooltip>
                      ) : (
                        <Toggle on={p.enabled} onChange={(v) => toggle(p.key, v)} aria-label={p.title} />
                      )}
                    </div>
                  </div>

                  {/* Category Badge & Tags */}
                  <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                    <Badge tone={categoryMeta.tone} className="text-[10px] uppercase tracking-wider font-semibold">
                      {t(categoryMeta.labelKey)}
                    </Badge>
                    {p.tags && p.tags.map((tag) => (
                      <span
                        key={tag}
                        onClick={() => setSearch("#" + tag)}
                        className="inline-flex items-center gap-0.5 px-1.5 py-0.5 text-[10px] rounded-md bg-foreground/[0.04] text-foreground/60 hover:text-accent-fg transition-colors cursor-pointer"
                      >
                        <Tag className="h-2.5 w-2.5" />
                        {tag}
                      </span>
                    ))}
                  </div>

                  {!p.core && lockedBy(p).length > 0 && (
                    <div className="mt-2 text-[10px] text-warning-fg font-medium">
                      {t("settings.pluginInUse", { plugin: p.title, dependents: lockedBy(p).join(", ") })}
                    </div>
                  )}
                  {/* Card Description */}
                  {description && (
                    <p className="text-xs text-foreground/60 leading-relaxed mt-2.5 line-clamp-3">
                      {description}
                    </p>
                  )}
                </div>

                {/* Card Bottom Meta Footer */}
                <div className="mt-4 pt-3 border-t border-border/40 flex items-center justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    {p.menus && p.menus.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {p.menus.slice(0, 2).map((m) => (
                          <span
                            key={m.id}
                            className="inline-block px-1.5 py-0.5 text-[10px] rounded-md bg-foreground/[0.05] text-foreground/70 truncate max-w-[120px]"
                          >
                            {m.label}
                          </span>
                        ))}
                        {p.menus.length > 2 && (
                          <span className="text-[10px] text-muted-foreground font-medium">
                            +{p.menus.length - 2}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-[10px] text-muted-foreground italic">{t("settings.backgroundService")}</span>
                    )}
                  </div>

                  {p.core ? (
                    <Badge tone="violet" className="text-[10px] shrink-0">
                      {t("settings.badgeAlwaysOn")}
                    </Badge>
                  ) : p.enabled ? (
                    <Badge tone="green" className="text-[10px] shrink-0">
                      {t("settings.badgeOn")}
                    </Badge>
                  ) : (
                    <Badge tone="neutral" className="text-[10px] shrink-0">
                      {t("settings.badgeOff")}
                    </Badge>
                  )}
                </div>
              </GlassCard>
            );
          })}
        </div>
      )}
    </div>
  );
}
