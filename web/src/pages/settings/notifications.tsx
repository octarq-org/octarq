import { ReactNode, useEffect, useState } from "react";
import { api, NotificationChannelType, NotificationChannel } from "../../api";
import { PageHeader, GlassCard, Button, Field, Toggle, Modal, toast } from "../../ui";
import { Bell, ChevronDown, Plus, Trash2, Send, Pencil } from "lucide-react";
import { useTranslation } from "../../i18n";
import { ExtensionSlot, NotificationChannelFormContext } from "../../plugin-sdk";
import { roleSatisfies, useCurrentRole } from "../../shell/role";

// ---------------------------------------------------------------------------
// ProviderRow — accordion per channel type; channels for that type listed
// inside the expanded body.
//
// Design decision: one row per type, channels nested.
// Auth providers are singletons so auth.tsx doesn't answer this. Here a
// workspace can have two Telegram bots, two webhook endpoints, etc. Grouping
// by type keeps the type-specific config form in one place and lets "Add
// another channel of this type" live naturally inside the row. A per-channel
// row grouped by type would require duplicating the form header for every
// channel, and the enabled/disabled badge would mean nothing at the type level.
// ---------------------------------------------------------------------------
function ProviderRow({
  type,
  title,
  description,
  icon,
  channels,
  onAdd,
  onEdit,
  onDelete,
  onTest,
  onToggle,
}: {
  type: string;
  title: string;
  description: string;
  icon?: ReactNode;
  channels: NotificationChannel[];
  onAdd: (type: string) => void;
  onEdit: (channel: NotificationChannel) => void;
  onDelete: (id: number) => void;
  onTest: (id: number) => void;
  onToggle: (channel: NotificationChannel) => void;
}) {
  const { t } = useTranslation();
  const { role, isInstanceAdmin } = useCurrentRole();
  const canManage = roleSatisfies("admin", role, isInstanceAdmin);
  const [open, setOpen] = useState(false);
  const hasChannels = channels.length > 0;
  const anyEnabled = channels.some((c) => c.enabled);
  const enabled = hasChannels && anyEnabled;

  return (
    <div className="border-b border-border last:border-b-0">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-3 px-4 py-3.5 text-left transition-colors hover:bg-surface-hover"
        aria-expanded={open}
      >
        {icon ? (
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
            {icon}
          </span>
        ) : (
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border bg-card">
            <Bell className="h-4 w-4 text-accent-fg" strokeWidth={1.75} />
          </span>
        )}
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">{title}</span>
          <span className="block truncate text-xs text-muted-foreground">{description}</span>
        </span>
        {hasChannels && (
          <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
            {channels.length}
          </span>
        )}
        <span
          className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${
            enabled
              ? "bg-success-fg/10 text-success-fg"
              : "bg-muted text-muted-foreground"
          }`}
        >
          {enabled
            ? t("settings.providerEnabled", "Enabled")
            : t("settings.providerDisabled", "Disabled")}
        </span>
        <ChevronDown
          className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open && (
        <div className="space-y-3 border-t border-border bg-well/50 px-4 py-4">
          {/* existing channels for this type */}
          {channels.length > 0 && (
            <div className="divide-y divide-foreground/[0.04] rounded-xl border border-foreground/[0.05] bg-well overflow-hidden">
              {channels.map((c) => (
                <div key={c.id} className="flex items-center gap-3 p-3 group">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="font-semibold text-sm text-foreground">{c.name}</span>
                      {!c.enabled && (
                        <span className="rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                          {t("settings.badgeDisabled")}
                        </span>
                      )}
                    </div>
                  </div>
                  {canManage && (
                    <div className="flex items-center gap-2 shrink-0">
                      <Button
                        variant="subtle"
                        onClick={() => onToggle(c)}
                        className="text-xs py-1 px-2.5"
                      >
                        {c.enabled ? t("settings.disable") : t("settings.enable")}
                      </Button>
                      <Button
                        variant="outline"
                        onClick={() => onTest(c.id)}
                        className="text-xs py-1 px-2.5 flex items-center gap-1"
                      >
                        <Send className="h-3 w-3" /> {t("settings.test")}
                      </Button>
                      <Button
                        variant="ghost"
                        onClick={() => onEdit(c)}
                        className="text-xs py-1 px-2.5"
                      >
                        <Pencil className="h-3 w-3" />
                      </Button>
                      <Button
                        variant="danger"
                        onClick={() => onDelete(c.id)}
                        className="text-xs py-1 px-2.5 border-0"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* add another channel of this type */}
          {canManage && (
            <Button
              variant="outline"
              onClick={() => onAdd(type)}
              className="mt-3 flex items-center gap-1.5 text-xs text-accent-fg hover:bg-surface-hover"
            >
              <Plus className="h-3.5 w-3.5" />
              {t("settings.notifyAddChannelOfType", "Add channel")}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// EditChannelModal — inline form modal reusing the slot mechanism.
// ---------------------------------------------------------------------------
function EditChannelModal({
  channel,
  initialType,
  onClose,
  onSaved,
}: {
  channel: NotificationChannel | null;
  initialType: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(channel?.name || "");
  const [enabled, setEnabled] = useState(channel ? channel.enabled : true);

  const initialCfg = channel?.id ? JSON.parse(channel.config) : {};
  const [config, setConfig] = useState<Record<string, any>>(initialCfg);

  const updateConfig = (key: string, value: any) =>
    setConfig((prev) => ({ ...prev, [key]: value }));

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function save() {
    setBusy(true);
    setError("");
    const configStr = JSON.stringify(config);
    try {
      if (channel?.id) {
        await api.updateNotificationChannel(channel.id, {
          name,
          type: initialType,
          config: configStr,
          enabled,
        });
      } else {
        await api.createNotificationChannel({
          name,
          type: initialType,
          config: configStr,
          enabled,
        });
      }
      onSaved();
    } catch (err: any) {
      setError(err.message || t("settings.failedToSave"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal
      title={channel ? t("settings.editAlertChannel") : t("settings.createAlertChannel")}
      onClose={onClose}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save();
        }}
        className="space-y-4"
      >
        <Field label={t("settings.channelName")} hint={t("settings.channelNameHint")}>
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Dev Team Telegram"
            required
            autoFocus
          />
        </Field>

        <NotificationChannelFormContext.Provider value={{ config, setConfig, updateConfig }}>
          <ExtensionSlot name={`settings-notification-channel:${initialType}`} />
        </NotificationChannelFormContext.Provider>

        {error && <div className="text-danger-fg text-xs font-semibold">{error}</div>}

        <div className="flex items-center gap-3 pt-2">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-foreground/60 select-none">{t("settings.channelEnabled")}</span>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-foreground/6">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("settings.cancel")}
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? t("settings.savingDots") : t("settings.saveChannel")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// NotificationChannels — the full page, accordion-style.
// ---------------------------------------------------------------------------
export function NotificationChannels() {
  const { t } = useTranslation();
  const [types, setTypes] = useState<NotificationChannelType[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(true);

  // editing state: { type, channel | null }
  const [editing, setEditing] = useState<{ type: string; channel: NotificationChannel | null } | null>(null);

  async function load() {
    setLoading(true);
    try {
      const [typs, chs] = await Promise.all([
        api.notificationChannelTypes(),
        api.notificationChannels(),
      ]);
      setTypes(typs);
      setChannels(chs);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleDelete(id: number) {
    if (!confirm(t("settings.confirmDeleteChannel"))) return;
    await api.deleteNotificationChannel(id);
    load();
  }

  async function handleTest(id: number) {
    try {
      await api.testNotificationChannel(id);
      toast.success(t("settings.testAlertSent"));
    } catch (err: any) {
      toast.error(t("settings.testFailed", { msg: err.message }));
    }
  }

  async function handleToggle(c: NotificationChannel) {
    await api.updateNotificationChannel(c.id, { enabled: !c.enabled });
    load();
  }

  // channels grouped by type
  const byType = (type: string) => channels.filter((c) => c.type === type);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.alertsTitle")}
        description={t("settings.alertsDescription")}
      />

      <div>
        <h3 className="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("settings.notifyProvidersHeading", "Notification providers")}
        </h3>
        <GlassCard className="overflow-hidden !p-0">
          {loading ? (
            <div className="py-10 text-center text-sm text-foreground/40">
              {t("settings.loadingLower")}
            </div>
          ) : types.length === 0 ? (
            <div className="flex flex-col items-center gap-2 py-10">
              <Bell className="h-8 w-8 text-foreground/30" />
              <div className="text-xs text-foreground/50">{t("settings.noChannels")}</div>
            </div>
          ) : (
            types.map((tp) => (
              <ProviderRow
                key={tp.type}
                type={tp.type}
                title={tp.title}
                description={tp.description}
                channels={byType(tp.type)}
                onAdd={(type) => setEditing({ type, channel: null })}
                onEdit={(c) => setEditing({ type: c.type, channel: c })}
                onDelete={handleDelete}
                onTest={handleTest}
                onToggle={handleToggle}
              />
            ))
          )}
        </GlassCard>
      </div>

      {editing && (
        <EditChannelModal
          channel={editing.channel}
          initialType={editing.type}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
    </div>
  );
}
