import React, { useEffect, useState } from "react";
import { Plus, RotateCcw, Save, Trash2, Sliders, Check } from "lucide-react";
import { useTranslation } from "../../i18n";
import { Button, GlassCard, PageHeader, ScreenWrap, Table, THead, TBody, TR, TH, TD, Toggle, cn, toast } from "../../ui";
import {
  useNotificationPreferencesQuery,
  useRegisteredChannelsQuery,
  useResetPreferencesMutation,
  useUpdatePreferencesMutation,
} from "./api";
import { PreferenceRouteRow } from "./types";

const DEFAULT_CATEGORIES: PreferenceRouteRow[] = [
  {
    key: "system",
    label: "系统事件",
    description: "系统升级、维护公告及实例运行状态通知",
    pattern: "system.*",
  },
  {
    key: "security",
    label: "安全与审计",
    description: "异地登录、密码重置、双因素认证与凭证变更",
    pattern: "security.*",
  },
  {
    key: "cron",
    label: "定时任务",
    description: "Cron 调度失败、周期作业异常及重试告警",
    pattern: "cron.*",
  },
  {
    key: "storage",
    label: "存储与备份",
    description: "存储容量上限警告、临时 Token 生成与备份快照",
    pattern: "storage.*",
  },
  {
    key: "fallback",
    label: "默认通用兜底",
    description: "未命中上述规则的所有事件默认通知渠道",
    pattern: "*",
  },
];

export function PreferencesPage() {
  const { t } = useTranslation();
  const { data: savedPreferences, isLoading: loadingPrefs } = useNotificationPreferencesQuery();
  const { data: channels = [], isLoading: loadingChannels } = useRegisteredChannelsQuery();

  const updateMutation = useUpdatePreferencesMutation();
  const resetMutation = useResetPreferencesMutation();

  // Local editable state: Record<eventPattern, string[] (channels)>
  const [matrix, setMatrix] = useState<Record<string, string[]>>(() => {
    const init: Record<string, string[]> = {};
    for (const cat of DEFAULT_CATEGORIES) {
      init[cat.pattern] = ["in_app", "email"];
    }
    return init;
  });
  const [customRows, setCustomRows] = useState<PreferenceRouteRow[]>([]);
  const [newPattern, setNewPattern] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [showAddModal, setShowAddModal] = useState(false);

  // Sync server preferences into local matrix state when data arrives
  useEffect(() => {
    if (!savedPreferences) return;

    const nextMatrix: Record<string, string[]> = {};
    const extraRows: PreferenceRouteRow[] = [];

    // Initialize defaults with ["in_app", "email"]
    for (const cat of DEFAULT_CATEGORIES) {
      nextMatrix[cat.pattern] = ["in_app", "email"];
    }

    // Apply saved server preferences
    for (const pref of savedPreferences) {
      nextMatrix[pref.eventPattern] = [...pref.channels];

      // If it's a custom pattern not in standard categories, add to customRows
      const isKnown = DEFAULT_CATEGORIES.some((c) => c.pattern === pref.eventPattern);
      if (!isKnown && pref.eventPattern) {
        extraRows.push({
          key: pref.eventPattern,
          label: pref.eventPattern,
          description: `自定义规则 (${pref.eventPattern})`,
          pattern: pref.eventPattern,
          isCustom: true,
        });
      }
    }

    setMatrix(nextMatrix);
    if (extraRows.length > 0) {
      setCustomRows(extraRows);
    }
  }, [savedPreferences]);

  const allRows = [...DEFAULT_CATEGORIES, ...customRows];

  // Toggle a channel for a specific pattern
  const toggleChannel = (pattern: string, channelName: string) => {
    setMatrix((prev) => {
      const current = prev[pattern] || [];
      const has = current.includes(channelName);
      const updated = has
        ? current.filter((c) => c !== channelName)
        : [...current, channelName];
      return { ...prev, [pattern]: updated };
    });
  };

  // Add custom pattern
  const handleAddCustomPattern = () => {
    const pat = newPattern.trim();
    if (!pat) return;
    if (allRows.some((r) => r.pattern === pat)) {
      toast.error(t("notifications.patternExists", "该规则已存在"));
      return;
    }

    const row: PreferenceRouteRow = {
      key: pat,
      label: newLabel.trim() || pat,
      description: `自定义规则 (${pat})`,
      pattern: pat,
      isCustom: true,
    };

    setCustomRows((prev) => [...prev, row]);
    setMatrix((prev) => ({ ...prev, [pat]: ["in_app"] }));
    setNewPattern("");
    setNewLabel("");
    setShowAddModal(false);
  };

  // Remove custom row
  const handleRemoveCustomRow = (pattern: string) => {
    setCustomRows((prev) => prev.filter((r) => r.pattern !== pattern));
    setMatrix((prev) => {
      const copy = { ...prev };
      delete copy[pattern];
      return copy;
    });
  };

  // Save preferences
  const handleSave = async () => {
    const payload = Object.entries(matrix).map(([eventPattern, chs]) => ({
      eventPattern,
      channels: chs,
    }));

    try {
      await updateMutation.mutateAsync(payload);
      toast.success(t("notifications.preferencesSaved", "通知偏好设置已保存"));
    } catch (err: any) {
      toast.error(err.message || t("notifications.saveFailed", "保存失败"));
    }
  };

  // Reset to default
  const handleReset = async () => {
    try {
      await resetMutation.mutateAsync();
      const defaultState: Record<string, string[]> = {};
      for (const cat of DEFAULT_CATEGORIES) {
        defaultState[cat.pattern] = ["in_app", "email"];
      }
      setMatrix(defaultState);
      setCustomRows([]);
      toast.success(t("notifications.preferencesReset", "已重置为默认通知偏好"));
    } catch (err: any) {
      toast.error(err.message || t("notifications.resetFailed", "重置失败"));
    }
  };

  const isLoading = loadingPrefs || loadingChannels;

  return (
    <ScreenWrap>
      <div className="space-y-6">
        <PageHeader
          title={t("notifications.preferencesPageTitle", "通知路由偏好设置")}
          description={t(
            "notifications.preferencesPageDescription",
            "按通知事件类型自定义分发渠道（站内信、邮件及插件扩展渠道）。",
          )}
          action={
            <div className="flex flex-wrap items-center gap-2.5">
              <Button
                variant="outline"
                size="sm"
                onClick={handleReset}
                disabled={resetMutation.isPending || isLoading}
                className="flex items-center gap-1.5 text-xs"
              >
                <RotateCcw className="h-3.5 w-3.5" />
                <span>{t("notifications.resetDefaults", "重置默认")}</span>
              </Button>

              <Button
                variant="primary"
                size="sm"
                onClick={handleSave}
                disabled={updateMutation.isPending || isLoading}
                className="flex items-center gap-1.5 text-xs"
              >
                <Save className="h-3.5 w-3.5" />
                <span>{t("notifications.savePreferences", "保存配置")}</span>
              </Button>
            </div>
          }
        />

        {/* Matrix Card */}
        <GlassCard className="overflow-hidden !p-0">
          <div className="border-b border-border bg-well/40 px-4 py-3 sm:px-6">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
              <div>
                <h3 className="text-sm font-semibold text-foreground flex items-center gap-2">
                  <Sliders className="h-4 w-4 text-primary" />
                  {t("notifications.matrixTitle", "事件渠道路由矩阵")}
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  {t("notifications.matrixHint", "每一行代表一类通知事件，勾选对应渠道即可自动投递。")}
                </p>
              </div>

              <Button
                variant="subtle"
                size="sm"
                onClick={() => setShowAddModal(true)}
                className="flex items-center gap-1.5 text-xs self-start sm:self-auto"
              >
                <Plus className="h-3.5 w-3.5" />
                <span>{t("notifications.addRule", "添加规则")}</span>
              </Button>
            </div>
          </div>

          {/* Matrix Table */}
          <Table className="text-xs sm:text-sm">
            <THead className="border-b border-border bg-well/20 text-muted-foreground font-medium">
              <TR>
                <TH className="py-3 px-4 sm:px-6 w-1/3 text-left">
                  {t("notifications.colEventType", "事件分类 / 规则")}
                </TH>
                {channels.map((ch) => (
                  <TH key={ch.name} className="py-3 px-4 text-center min-w-[100px]">
                    <div className="flex flex-col items-center">
                      <span className="font-semibold text-foreground">{ch.displayName || ch.name}</span>
                      <span className="text-[10px] text-muted-foreground font-mono">({ch.name})</span>
                    </div>
                  </TH>
                ))}
                <TH className="py-3 px-4 w-12 text-center" />
              </TR>
            </THead>

            <TBody className="divide-y divide-border">
              {isLoading ? (
                <TR>
                  <TD colSpan={channels.length + 2} className="py-12 text-center text-muted-foreground">
                    {t("notifications.loading", "加载中...")}
                  </TD>
                </TR>
              ) : (
                allRows.map((row) => {
                  const rowChannels = matrix[row.pattern] || [];
                  return (
                    <TR key={row.pattern} className="hover:bg-surface-hover/40 transition-colors">
                      <TD className="py-3.5 px-4 sm:px-6">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-foreground">{row.label}</span>
                          <code className="rounded bg-muted px-1.5 py-0.5 text-[11px] font-mono text-muted-foreground">
                            {row.pattern}
                          </code>
                        </div>
                        <p className="text-xs text-muted-foreground mt-0.5">{row.description}</p>
                      </TD>

                      {channels.map((ch) => {
                        const isEnabled = rowChannels.includes(ch.name);
                        return (
                          <TD key={ch.name} className="py-3.5 px-4 text-center">
                            <button
                              type="button"
                              role="switch"
                              aria-checked={isEnabled}
                              onClick={() => toggleChannel(row.pattern, ch.name)}
                              className={cn(
                                "inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2",
                                isEnabled ? "bg-primary" : "bg-muted-foreground/25",
                              )}
                            >
                              <span
                                className={cn(
                                  "pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-md ring-0 transition duration-200 ease-in-out",
                                  isEnabled ? "translate-x-5" : "translate-x-0",
                                )}
                              />
                            </button>
                          </TD>
                        );
                      })}

                      <TD className="py-3.5 px-4 text-center">
                        {row.isCustom && (
                          <button
                            type="button"
                            onClick={() => handleRemoveCustomRow(row.pattern)}
                            className="text-muted-foreground hover:text-danger-fg transition-colors p-1"
                            title={t("notifications.delete", "删除")}
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        )}
                      </TD>
                    </TR>
                  );
                })
              )}
            </TBody>
          </Table>
        </GlassCard>

        {/* Add custom rule dialog */}
        {showAddModal && (
          <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40 backdrop-blur-xs">
            <div className="w-full max-w-md rounded-2xl border border-border bg-background p-6 shadow-xl space-y-4 animate-in zoom-in-95">
              <h3 className="text-base font-semibold text-foreground">
                {t("notifications.addRuleTitle", "添加自定义通知路由规则")}
              </h3>

              <div className="space-y-3">
                <div>
                  <label className="block text-xs font-medium text-foreground mb-1">
                    {t("notifications.ruleName", "规则名称")}
                  </label>
                  <input
                    type="text"
                    value={newLabel}
                    onChange={(e) => setNewLabel(e.target.value)}
                    placeholder={t("notifications.ruleNamePlaceholder", "如：支付结算通知")}
                    className="w-full rounded-xl border border-border bg-surface-hover/30 px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                </div>

                <div>
                  <label className="block text-xs font-medium text-foreground mb-1">
                    {t("notifications.rulePattern", "事件模式匹配通配符")}
                  </label>
                  <input
                    type="text"
                    value={newPattern}
                    onChange={(e) => setNewPattern(e.target.value)}
                    placeholder={t("notifications.rulePatternPlaceholder", "如：billing.* 或 webhook.failed")}
                    className="w-full rounded-xl border border-border bg-surface-hover/30 px-3 py-2 text-sm font-mono text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
                  />
                  <p className="text-[11px] text-muted-foreground mt-1">
                    {t("notifications.rulePatternHelp", "支持如 billing.*、*.failed 或精确事件名称。")}
                  </p>
                </div>
              </div>

              <div className="flex items-center justify-end gap-2 pt-2 border-t border-border">
                <Button variant="ghost" size="sm" onClick={() => setShowAddModal(false)}>
                  {t("common.cancel", "取消")}
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  disabled={!newPattern.trim()}
                  onClick={handleAddCustomPattern}
                >
                  {t("common.confirm", "添加")}
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>
    </ScreenWrap>
  );
}
