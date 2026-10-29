import {
  Button,
  Field,
  GlassCard,
  Input,
  Modal,
  PageHeader,
  ScreenWrap,
  Switch,
  Table,
  THead,
  TBody,
  TR,
  TH,
  TD,
  toast,
} from "@octarq/plugin-sdk";
import React, { useEffect, useMemo, useState } from "react";
import { Plus, RotateCcw, Save, Trash2, Sliders } from "lucide-react";
import { useTranslation, type TFunc } from "../../i18n";
import {
  useNotificationPreferencesQuery,
  useRegisteredChannelsQuery,
  useResetPreferencesMutation,
  useUpdatePreferencesMutation,
} from "./api";
import { PreferenceRouteRow } from "./types";

// The categories the matrix always shows. Labels and descriptions come from the
// page dictionary at render time; only the routing pattern is fixed here.
const DEFAULT_CATEGORIES = [
  {
    pattern: "system.*",
    labelKey: "notifications.catSystemLabel",
    descKey: "notifications.catSystemDesc",
    fallbackLabel: "System events",
    fallbackDesc: "Upgrades, maintenance notices, and instance status.",
  },
  {
    pattern: "security.*",
    labelKey: "notifications.catSecurityLabel",
    descKey: "notifications.catSecurityDesc",
    fallbackLabel: "Security and audit",
    fallbackDesc: "Remote sign-in, password reset, two-factor, and credential changes.",
  },
  {
    pattern: "cron.*",
    labelKey: "notifications.catCronLabel",
    descKey: "notifications.catCronDesc",
    fallbackLabel: "Scheduled jobs",
    fallbackDesc: "Cron failures, recurring job errors, and retry alerts.",
  },
  {
    pattern: "storage.*",
    labelKey: "notifications.catStorageLabel",
    descKey: "notifications.catStorageDesc",
    fallbackLabel: "Storage and backup",
    fallbackDesc: "Capacity limits, temporary tokens, and backup snapshots.",
  },
  {
    pattern: "*",
    labelKey: "notifications.catFallbackLabel",
    descKey: "notifications.catFallbackDesc",
    fallbackLabel: "Everything else",
    fallbackDesc: "Default channels for events that match no other rule.",
  },
] as const;

const DEFAULT_PATTERNS: string[] = DEFAULT_CATEGORIES.map((c) => c.pattern);

function defaultRows(t: TFunc): PreferenceRouteRow[] {
  return DEFAULT_CATEGORIES.map((c) => ({
    key: c.pattern,
    pattern: c.pattern,
    label: t(c.labelKey, c.fallbackLabel),
    description: t(c.descKey, c.fallbackDesc),
  }));
}

export function PreferencesPage({ embedded = false }: { embedded?: boolean } = {}) {
  const { t } = useTranslation();
  const {
    data: savedPreferences,
    isLoading: loadingPrefs,
    isError: prefsLoadFailed,
    refetch: retryPrefs,
  } = useNotificationPreferencesQuery();
  const { data: channels = [], isLoading: loadingChannels } = useRegisteredChannelsQuery();

  const updateMutation = useUpdatePreferencesMutation();
  const resetMutation = useResetPreferencesMutation();

  // Local editable state: Record<eventPattern, string[] (channels)>
  const [matrix, setMatrix] = useState<Record<string, string[]>>(() => {
    const init: Record<string, string[]> = {};
    for (const pattern of DEFAULT_PATTERNS) init[pattern] = ["in_app", "email"];
    return init;
  });
  const [customRows, setCustomRows] = useState<PreferenceRouteRow[]>([]);
  const [newPattern, setNewPattern] = useState("");
  const [newLabel, setNewLabel] = useState("");
  const [showAddModal, setShowAddModal] = useState(false);

  // Sync server preferences into local matrix state when data arrives.
  //
  // The effect must not depend on `t`: `t` changes identity on every language
  // switch, and re-running this effect then rebuilds the matrix from the last
  // server response, silently discarding edits the user has not saved. Custom
  // rows therefore store no description — it is derived at render.
  useEffect(() => {
    if (!savedPreferences) return;

    const nextMatrix: Record<string, string[]> = {};
    const extraRows: PreferenceRouteRow[] = [];

    for (const pattern of DEFAULT_PATTERNS) nextMatrix[pattern] = ["in_app", "email"];

    // Apply saved server preferences
    for (const pref of savedPreferences) {
      nextMatrix[pref.eventPattern] = [...pref.channels];

      // If it's a custom pattern not in standard categories, add to customRows
      const isKnown = DEFAULT_PATTERNS.includes(pref.eventPattern);
      if (!isKnown && pref.eventPattern) {
        extraRows.push({
          key: pref.eventPattern,
          label: pref.eventPattern,
          description: "",
          pattern: pref.eventPattern,
          isCustom: true,
        });
      }
    }

    setMatrix(nextMatrix);
    // Replace, never merge: a rule deleted here (or in another tab) must not
    // linger as a ghost row that the next Save would re-create.
    setCustomRows(extraRows);
  }, [savedPreferences]);

  const allRows = useMemo(
    () => [
      ...defaultRows(t),
      ...customRows.map((row) => ({
        ...row,
        description: t("notifications.customRuleDesc", "Custom rule ({{pattern}})", { pattern: row.pattern }),
      })),
    ],
    [t, customRows],
  );

  // Toggle a channel for a specific pattern
  const toggleChannel = (pattern: string, channelName: string) => {
    setMatrix((prev) => {
      const current = prev[pattern] || [];
      const has = current.includes(channelName);
      const updated = has ? current.filter((c) => c !== channelName) : [...current, channelName];
      return { ...prev, [pattern]: updated };
    });
  };

  // Add custom pattern
  const handleAddCustomPattern = () => {
    const pat = newPattern.trim();
    if (!pat) return;
    if (allRows.some((r) => r.pattern === pat)) {
      toast.error(t("notifications.patternExists", "This event pattern already exists"));
      return;
    }

    const row: PreferenceRouteRow = {
      key: pat,
      label: newLabel.trim() || pat,
      description: "",
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
      toast.success(t("notifications.preferencesSaved", "Notification preferences saved"));
    } catch (err: any) {
      toast.error(err.message || t("notifications.saveFailed", "Could not save notification preferences. Try again."));
    }
  };

  // Reset to default
  const handleReset = async () => {
    try {
      await resetMutation.mutateAsync();
      const defaultState: Record<string, string[]> = {};
      for (const pattern of DEFAULT_PATTERNS) defaultState[pattern] = ["in_app", "email"];
      setMatrix(defaultState);
      setCustomRows([]);
      toast.success(t("notifications.preferencesReset", "Notification preferences reset to defaults"));
    } catch (err: any) {
      toast.error(err.message || t("notifications.resetFailed", "Could not reset notification preferences. Try again."));
    }
  };

  const isLoading = loadingPrefs || loadingChannels;

  // A failed load leaves the matrix at its defaults. Saving then would write
  // those defaults over the user's real routing, so both actions stay off and
  // the matrix is replaced by the error state.
  const actionsDisabled = isLoading || prefsLoadFailed;

  const deleteRuleButton = (row: PreferenceRouteRow) =>
    row.isCustom && (
      <button
        type="button"
        onClick={() => handleRemoveCustomRow(row.pattern)}
        aria-label={`${t("notifications.delete", "Delete")} — ${row.label}`}
        className="tap-expand-sm flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-hover hover:text-danger-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border"
      >
        <Trash2 className="h-4 w-4" aria-hidden="true" />
      </button>
    );

  // gap-3 keeps the expanded touch targets of neighbouring buttons apart.
  const actionButtons = (
    <div className="flex flex-wrap items-center gap-3">
      <Button
        variant="outline"
        size="sm"
        onClick={handleReset}
        disabled={resetMutation.isPending || actionsDisabled}
        className="tap-expand-sm flex items-center gap-1.5 text-xs"
      >
        <RotateCcw className="h-3.5 w-3.5" />
        <span>{t("notifications.resetDefaults", "Reset to default")}</span>
      </Button>

      <Button
        variant="primary"
        size="sm"
        onClick={handleSave}
        disabled={updateMutation.isPending || actionsDisabled}
        className="tap-expand-sm flex items-center gap-1.5 text-xs"
      >
        <Save className="h-3.5 w-3.5" />
        <span>{t("notifications.savePreferences", "Save preferences")}</span>
      </Button>
    </div>
  );

  const mainContent = (
    <div className="space-y-6">
      {!embedded ? (
        <PageHeader
          title={t("personal.notificationsTitle", "Personal Notifications")}
          description={t(
            "personal.notificationsDesc",
            "Manage event subscriptions, mute channels, and configure personal notification routing.",
          )}
          action={actionButtons}
        />
      ) : (
        <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
          <p className="text-xs text-muted-foreground">
            {t(
              "personal.notificationsDesc",
              "Manage event subscriptions, mute channels, and configure personal notification routing.",
            )}
          </p>
          {actionButtons}
        </div>
      )}

      {/* Matrix Card */}
      <GlassCard className="overflow-hidden !p-0">
        <div className="border-b border-border bg-well px-4 py-3 sm:px-6">
          <div className="flex flex-col justify-between gap-2 sm:flex-row sm:items-center">
            <div>
              <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <Sliders className="h-4 w-4 text-accent-fg" aria-hidden="true" />
                {t("notifications.matrixTitle", "Event Route Matrix")}
              </h3>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t("notifications.matrixHint", "Each row represents an event category. Toggle channels to enable delivery.")}
              </p>
            </div>

            <Button
              variant="subtle"
              size="sm"
              onClick={() => setShowAddModal(true)}
              disabled={prefsLoadFailed}
              className="tap-expand-sm flex items-center gap-1.5 self-start text-xs sm:self-auto"
            >
              <Plus className="h-3.5 w-3.5" />
              <span>{t("notifications.addRule", "Add rule")}</span>
            </Button>
          </div>
        </div>

        {prefsLoadFailed ? (
          <div className="px-4 py-10 text-center">
            <p className="text-sm text-foreground">
              {t("notifications.loadFailed", "Could not load notifications. Try again.")}
            </p>
            <Button variant="outline" size="sm" onClick={() => retryPrefs()} className="mt-3">
              {t("notifications.refresh", "Refresh")}
            </Button>
          </div>
        ) : isLoading ? (
          <p className="py-12 text-center text-xs text-muted-foreground">{t("notifications.loading", "Loading…")}</p>
        ) : (
          <>
            {/* Phone: one card per event, so the channel columns never scroll
                sideways inside the card. */}
            <ul className="divide-y divide-border sm:hidden">
              {allRows.map((row) => {
                const rowChannels = matrix[row.pattern] || [];
                return (
                  <li key={row.pattern} className="px-4 py-3.5">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium text-foreground">{row.label}</span>
                          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                            {row.pattern}
                          </code>
                        </div>
                        <p className="mt-0.5 text-xs text-muted-foreground">{row.description}</p>
                      </div>
                      {deleteRuleButton(row)}
                    </div>

                    {/* Each row is a full 44px tall, so the switch's expanded
                        touch target on a coarse pointer stays inside its own
                        row. Tighter rows overlap, and the lower switch then
                        takes taps meant for the upper one. */}
                    <div className="mt-2 space-y-1">
                      {channels.map((ch) => (
                        <div key={ch.name} className="flex min-h-11 items-center justify-between gap-3">
                          <span className="text-xs text-muted-foreground">
                            {ch.displayName || ch.name}
                          </span>
                          <Switch
                            checked={rowChannels.includes(ch.name)}
                            onCheckedChange={() => toggleChannel(row.pattern, ch.name)}
                            aria-label={`${row.label} — ${ch.displayName || ch.name}`}
                          />
                        </div>
                      ))}
                    </div>
                  </li>
                );
              })}
            </ul>

            {/* Tablet and up: the matrix. */}
            <div className="hidden sm:block">
              <Table className="text-xs sm:text-sm">
                <THead className="border-b border-border bg-well font-medium text-muted-foreground">
                  <TR>
                    <TH className="w-1/3 px-4 py-3 text-left sm:px-6">
                      {t("notifications.colEventType", "Event Category / Pattern")}
                    </TH>
                    {channels.map((ch) => (
                      <TH key={ch.name} className="min-w-[100px] px-4 py-3 text-center">
                        <div className="flex flex-col items-center">
                          <span className="font-semibold text-foreground">{ch.displayName || ch.name}</span>
                          <span className="font-mono text-[10px] text-muted-foreground">({ch.name})</span>
                        </div>
                      </TH>
                    ))}
                    <TH className="w-12 px-4 py-3 text-center" />
                  </TR>
                </THead>

                <TBody className="divide-y divide-border">
                  {allRows.map((row) => {
                    const rowChannels = matrix[row.pattern] || [];
                    return (
                      <TR key={row.pattern} className="transition-colors hover:bg-surface-hover/40">
                        <TD className="px-4 py-3.5 sm:px-6">
                          <div className="flex items-center gap-2">
                            <span className="font-medium text-foreground">{row.label}</span>
                            <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
                              {row.pattern}
                            </code>
                          </div>
                          <p className="mt-0.5 text-xs text-muted-foreground">{row.description}</p>
                        </TD>

                        {channels.map((ch) => (
                          <TD key={ch.name} className="px-4 py-3.5 text-center">
                            <Switch
                              checked={rowChannels.includes(ch.name)}
                              onCheckedChange={() => toggleChannel(row.pattern, ch.name)}
                              aria-label={`${row.label} — ${ch.displayName || ch.name}`}
                            />
                          </TD>
                        ))}

                        <TD className="px-4 py-3.5 text-center">{deleteRuleButton(row)}</TD>
                      </TR>
                    );
                  })}
                </TBody>
              </Table>
            </div>
          </>
        )}
      </GlassCard>

      {showAddModal && (
        <Modal title={t("notifications.addRuleTitle", "Add Custom Event Rule")} onClose={() => setShowAddModal(false)}>
          <div className="space-y-4">
            <Field label={t("notifications.ruleName", "Rule Name")}>
              <Input
                value={newLabel}
                onChange={(e) => setNewLabel(e.target.value)}
                placeholder={t("notifications.ruleNamePlaceholder", "e.g. Payment notices")}
              />
            </Field>

            <Field
              label={t("notifications.rulePattern", "Event Pattern Wildcard")}
              hint={t("notifications.rulePatternHelp", "Supports patterns like billing.*, *.failed, or exact event names.")}
            >
              <Input
                className="font-mono"
                value={newPattern}
                onChange={(e) => setNewPattern(e.target.value)}
                placeholder={t("notifications.rulePatternPlaceholder", "e.g. billing.* or webhook.failed")}
              />
            </Field>

            <div className="flex items-center justify-end gap-2 border-t border-border pt-2">
              <Button variant="ghost" size="sm" onClick={() => setShowAddModal(false)}>
                {t("common.cancel", "Cancel")}
              </Button>
              <Button variant="primary" size="sm" disabled={!newPattern.trim()} onClick={handleAddCustomPattern}>
                {t("notifications.addRule", "Add rule")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );

  if (embedded) return mainContent;
  return <ScreenWrap>{mainContent}</ScreenWrap>;
}
