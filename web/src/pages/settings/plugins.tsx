import { useEffect, useState } from "react";
import { api, ApiError, PluginInfo } from "../../api";
import { Toggle, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { ShieldAlert, Puzzle, Search, CheckCircle2, XCircle } from "lucide-react";
import { useTranslation } from "../../i18n";
import { menuIcon } from "../../shell/areas";

// Helper component to render a plugin's icon or custom logo.
// Supports: Image URL (http/https/data/relative), Lucide icon key (resolved via menuIcon), emoji, or fallback Puzzle icon.
function PluginIcon({ iconStr, firstMenuIcon }: { iconStr?: string; firstMenuIcon?: string }) {
  const target = iconStr || firstMenuIcon;
  if (!target) {
    return <Puzzle className="h-5 w-5 text-accent-fg" />;
  }

  // Check if target is an image URL or data URI
  if (target.startsWith("http://") || target.startsWith("https://") || target.startsWith("data:") || target.startsWith("/")) {
    return <img src={target} alt="Plugin logo" className="h-6 w-6 object-contain rounded-lg" />;
  }

  // Resolve via single menuIcon map in shell/areas
  const IconComp = menuIcon(target);
  if (IconComp) {
    return <IconComp className="h-5 w-5 text-accent-fg" />;
  }

  // Render text / emoji
  return <span className="text-lg leading-none select-none">{target}</span>;
}

export function PluginsSettings() {
  const { t } = useTranslation();
  const [plugins, setPlugins] = useState<PluginInfo[] | null>(null);
  const [err, setErr] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "enabled" | "disabled">("all");

  function load() {
    api.plugins().then(setPlugins).catch((e: ApiError) => setErr(e.message || t("settings.failedLoadPlugins")));
  }
  useEffect(load, []);

  async function toggle(key: string, enabled: boolean) {
    setErr("");
    // optimistic update; revert on failure
    setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled } : p)) ?? prev);
    try {
      await api.updatePlugin(key, enabled);
      window.dispatchEvent(new CustomEvent("octarq:plugins-changed"));
    } catch (e) {
      setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled: !enabled } : p)) ?? prev);
      setErr(e instanceof ApiError ? e.message : t("settings.failedUpdatePlugin"));
    }
  }

  const filteredPlugins = (plugins ?? []).filter((p) => {
    const title = p.title.toLowerCase();
    const key = p.key.toLowerCase();
    const desc = (p.description || "").toLowerCase();
    const query = search.toLowerCase().trim();

    const matchesSearch = !query || title.includes(query) || key.includes(query) || desc.includes(query);
    const matchesFilter =
      statusFilter === "all" ||
      (statusFilter === "enabled" && p.enabled) ||
      (statusFilter === "disabled" && !p.enabled);

    return matchesSearch && matchesFilter;
  });

  const enabledCount = (plugins ?? []).filter((p) => p.enabled).length;

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.pluginsTitle")} description={t("settings.pluginsDescription")} />

      {err && (
        <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-danger-fg text-xs flex gap-2 items-center">
          <ShieldAlert className="h-4 w-4 shrink-0" />
          <span>{err}</span>
        </div>
      )}

      {plugins !== null && plugins.length > 0 && (
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-foreground/[0.02] p-3 rounded-2xl border border-border/60">
          {/* Search Bar */}
          <div className="relative flex-1 max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("common.search") || "Search plugins..."}
              className="w-full pl-9 pr-3 py-1.5 text-xs rounded-xl bg-card border border-border focus:outline-none focus:ring-2 focus:ring-accent-fg/30 text-foreground placeholder:text-muted-foreground"
            />
          </div>

          {/* Filter Segmented Control */}
          <div className="flex items-center gap-1 bg-surface rounded-xl p-1 border border-border/40 text-xs">
            <button
              onClick={() => setStatusFilter("all")}
              className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                statusFilter === "all"
                  ? "bg-card text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              All ({plugins.length})
            </button>
            <button
              onClick={() => setStatusFilter("enabled")}
              className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                statusFilter === "enabled"
                  ? "bg-card text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              Active ({enabledCount})
            </button>
            <button
              onClick={() => setStatusFilter("disabled")}
              className={`px-3 py-1 rounded-lg transition-colors font-medium ${
                statusFilter === "disabled"
                  ? "bg-card text-foreground shadow-xs"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              Disabled ({plugins.length - enabledCount})
            </button>
          </div>
        </div>
      )}

      {plugins === null ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">{t("settings.loadingPlugins")}</GlassCard>
      ) : plugins.length === 0 ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/55">{t("settings.noPlugins")}</GlassCard>
      ) : filteredPlugins.length === 0 ? (
        <GlassCard className="p-8 text-sm text-center text-foreground/50">
          No plugins match your current search or filter.
        </GlassCard>
      ) : (
        /* Card Grid Layout */
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredPlugins.map((p) => {
            const description = t("settings.pluginDesc." + p.key, p.description || "");
            const firstMenuIcon = p.menus?.[0]?.icon;

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
                        <h3 className="text-sm font-bold text-foreground truncate">{p.title}</h3>
                        <span className="text-[10px] font-mono text-muted-foreground block truncate">@{p.key}</span>
                      </div>
                    </div>

                    <Toggle on={p.enabled} onChange={(v) => toggle(p.key, v)} />
                  </div>

                  {/* Card Description */}
                  {description && (
                    <p className="text-xs text-foreground/60 leading-relaxed mt-3 line-clamp-3">
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
                      <span className="text-[10px] text-muted-foreground italic">Background Service</span>
                    )}
                  </div>

                  {p.enabled ? (
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
