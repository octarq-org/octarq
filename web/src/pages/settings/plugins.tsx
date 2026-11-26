import { Switch, PageShell, PageCard, PageEmpty, Badge, Alert, Tooltip, confirmDialog, Input, TabBar, type BadgeTone } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { api, ApiError, PluginCategoryInfo, PluginInfo } from "../../api";
import { ShieldAlert, Puzzle, Search, Tag, Info } from "lucide-react";
import { useTranslation } from "../../i18n";
import { menuIcon } from "../../shell/areas";
import { useSettingsData } from "./shared";

const BADGE_TONES = new Set<BadgeTone>([
  "default",
  "info",
  "success",
  "warning",
  "danger",
  "review",
  "secondary",
  "outline",
  "indigo",
  "violet",
  "green",
  "amber",
  "red",
  "cyan",
  "neutral",
]);

function PluginIcon({ iconStr, firstMenuIcon }: { iconStr?: string; firstMenuIcon?: string }) {
  const { t } = useTranslation();
  const target = iconStr || firstMenuIcon;
  if (!target) {
    return <Puzzle className="h-5 w-5 text-accent-fg" />;
  }

  if (target.startsWith("http://") || target.startsWith("https://") || target.startsWith("data:") || target.startsWith("/")) {
    return <img src={target} alt={t("settings.pluginLogoAlt")} width={24} height={24} className="h-6 w-6 object-contain rounded-lg" />;
  }

  const IconComp = menuIcon(target);
  if (IconComp) {
    return <IconComp className="h-5 w-5 text-accent-fg" />;
  }

  return <span className="text-lg leading-none select-none">{target}</span>;
}

function lockedBy(p: PluginInfo): string[] {
  return p.enabled ? (p.requiredBy ?? []) : [];
}

export function PluginsSettings() {
  const { s: settings } = useSettingsData();
  const { t, lang } = useTranslation();
  const [plugins, setPlugins] = useState<PluginInfo[] | null>(null);
  const [categories, setCategories] = useState<PluginCategoryInfo[]>([]);
  const [err, setErr] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "enabled" | "disabled">("all");
  const [categoryFilter, setCategoryFilter] = useState<string>("all");

  const categoryMap: Record<string, PluginCategoryInfo> = {};
  for (const c of categories) {
    categoryMap[c.key] = c;
  }
  const categoryLabel = (key: string) => categoryMap[key]?.labels?.[lang] || key;
  const categoryTone = (key: string): BadgeTone => {
    const tone = categoryMap[key]?.tone as BadgeTone | undefined;
    return tone && BADGE_TONES.has(tone) ? tone : "neutral";
  };

  function load() {
    api.plugins()
      .then((res) => {
        setPlugins(res.plugins);
        setCategories(res.categories ?? []);
      })
      .catch((e: ApiError) => setErr(e.message || t("settings.failedLoadPlugins")));
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
    
    setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled } : p)) ?? prev);
    try {
      await api.updatePlugin(key, enabled);
      load();
      window.dispatchEvent(new CustomEvent("octarq:plugins-changed"));
    } catch (e) {
      setPlugins((prev) => prev?.map((p) => (p.key === key ? { ...p, enabled: !enabled } : p)) ?? prev);
      if (e instanceof ApiError && e.status === 409) {
        let msg = e.message;
        const dependents = (e.body as { dependents?: string[] } | undefined)?.dependents;
        if (dependents) {
          msg = t("settings.pluginInUse", { plugin: key, dependents: dependents.join(", ") });
        }
        setErr(msg);
      } else {
        setErr(e instanceof ApiError ? e.message : t("settings.failedUpdatePlugin"));
      }
    }
  }

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
    <PageShell
      title={t("settings.pluginsTitle")}
      description={t("settings.pluginsDescription")}
    >
      <Alert variant="info" icon={<Info className="h-4 w-4 shrink-0" />} className="mt-6 text-xs p-3 rounded-lg">
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
        <Alert variant="danger" icon={<ShieldAlert className="h-4 w-4 shrink-0" />} className="mt-4 text-xs p-3 rounded-lg">
          {err}
        </Alert>
      )}

      {plugins !== null && plugins.length > 0 && (
        <div className="mt-6 space-y-3">
          <div className="overflow-x-auto pb-1">
            <TabBar
              size="sm"
              value={categoryFilter}
              onValueChange={setCategoryFilter}
              items={[
                {
                  value: "all",
                  label: t("settings.allCategories"),
                  count: plugins.length,
                },
                ...availableCategories.map((c) => {
                  const count = (plugins ?? []).filter((p) => (p.category || "utilities") === c).length;
                  return {
                    value: c,
                    label: <span className="capitalize">{categoryLabel(c)}</span>,
                    count,
                  };
                }),
              ]}
            />
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="relative w-full max-w-md">
              <Search className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                aria-label={t("common.search")}
                placeholder={t("common.search")}
                className="!pl-10"
              />
            </div>

            <TabBar
              size="sm"
              value={statusFilter}
              onValueChange={(v) => setStatusFilter(v as typeof statusFilter)}
              items={[
                { value: "all", label: t("settings.filterAll"), count: plugins.length },
                { value: "enabled", label: t("settings.enabled"), count: enabledCount },
                { value: "disabled", label: t("settings.disabled"), count: plugins.length - enabledCount },
              ]}
            />
          </div>
        </div>
      )}

      {plugins === null ? (
        <PageCard className="p-8 text-sm text-center text-muted-foreground">{t("settings.loadingPlugins")}</PageCard>
      ) : plugins.length === 0 ? (
        <PageEmpty title={t("settings.noPlugins")} />
      ) : filteredPlugins.length === 0 ? (
        <PageEmpty title={t("settings.noMatchingPlugins")} />
      ) : (
        <div className="mt-6 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredPlugins.map((p) => {
            const description = t("settings.pluginDesc." + p.key, p.description || "");
            const firstMenuIcon = p.menus?.[0]?.icon;
            const catKey = p.category || "utilities";

            return (
              <PageCard
                key={p.key}
                className={`mt-0 p-5 flex flex-col justify-between transition-colors duration-150 hover:border-accent-fg/40 ${
                  p.enabled ? "border-border bg-card" : "opacity-80"
                }`}
              >
                <div>
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-well border border-border">
                        <PluginIcon iconStr={p.icon} firstMenuIcon={firstMenuIcon} />
                      </div>
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <h3 className="text-sm font-semibold text-foreground truncate">{p.title}</h3>
                          {p.core && (
                            <Badge tone="violet" className="shrink-0 text-[10px] uppercase tracking-wider font-semibold">
                              {t("settings.pluginCoreBadge")}
                            </Badge>
                          )}
                        </div>
                        <span className="text-[10px] font-mono text-muted-foreground block truncate">@{p.key}</span>
                      </div>
                    </div>

                    <div className="flex flex-col items-end gap-1">
                      {p.core ? (
                        <Tooltip content={t("settings.pluginCoreHint")}>
                          <Switch checked onCheckedChange={() => {}} disabled aria-label={p.title} />
                        </Tooltip>
                      ) : lockedBy(p).length > 0 ? (
                        <Tooltip
                          content={t("settings.pluginInUse", { plugin: p.title, dependents: lockedBy(p).join(", ") })}
                        >
                          <Switch checked={p.enabled} onCheckedChange={() => {}} disabled aria-label={p.title} />
                        </Tooltip>
                      ) : (
                        <Switch checked={p.enabled} onCheckedChange={(v) => toggle(p.key, v)} aria-label={p.title} />
                      )}
                    </div>
                  </div>

                  <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                    <Badge tone={categoryTone(catKey)} className="text-[10px] uppercase tracking-wider font-semibold">
                      {categoryLabel(catKey)}
                    </Badge>
                    {p.tags && p.tags.map((tag) => (
                      <span
                        key={tag}
                        onClick={() => setSearch("#" + tag)}
                        className="inline-flex items-center gap-0.5 px-1.5 py-0.5 text-[10px] rounded-md bg-well text-muted-foreground hover:text-accent-fg transition-colors cursor-pointer"
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
                  {description && (
                    <p className="text-xs text-muted-foreground leading-relaxed mt-2.5 line-clamp-3">
                      {description}
                    </p>
                  )}
                </div>

                <div className="mt-4 pt-3 border-t border-border flex items-center justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    {p.menus && p.menus.length > 0 ? (
                      <div className="flex flex-wrap gap-1">
                        {p.menus.slice(0, 2).map((m) => (
                          <span
                            key={m.id}
                            className="inline-block px-1.5 py-0.5 text-[10px] rounded-md bg-well text-muted-foreground truncate max-w-[120px]"
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
              </PageCard>
            );
          })}
        </div>
      )}
    </PageShell>
  );
}
