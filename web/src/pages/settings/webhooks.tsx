import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, Overview, PluginInfo, WebhookEventGroup } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button, Select, toast } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import { useSettingsData, SavedBadge } from "./shared";

export function WebhooksSettings() {
  const { t } = useTranslation();
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [show, setShow] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [all, setAll] = useState(true);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [eventGroups, setEventGroups] = useState<WebhookEventGroup[] | null>(null);
  const [busy, setBusy] = useState(false);

  function load() { api.webhooks().then(setWebhooks).catch(() => {}); }
  useEffect(load, []);
  useEffect(() => { api.webhookEvents().then(setEventGroups).catch(() => setEventGroups([])); }, []);

  // key → definition, for rendering a stored subscription string as titled badges.
  const defByKey = new Map((eventGroups ?? []).flatMap((g) => g.events.map((e) => [e.key, e] as const)));

  function toggleEvent(key: string, on: boolean) {
    setSelected((prev) => { const next = new Set(prev); if (on) next.add(key); else next.delete(key); return next; });
  }
  function toggleGroup(g: WebhookEventGroup, on: boolean) {
    setSelected((prev) => { const next = new Set(prev); for (const e of g.events) { if (on) next.add(e.key); else next.delete(e.key); } return next; });
  }

  async function del(id: number) { if (!confirm(t("settings.confirmDeleteWebhook"))) return; await api.deleteWebhook(id); setWebhooks((w) => w.filter((h) => h.id !== id)); }
  async function toggle(h: any) { const u = await api.updateWebhook(h.id, { enabled: !h.enabled }); setWebhooks((w) => w.map((x) => x.id === h.id ? u : x)); }
  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    setBusy(true);
    try {
      const events = all || selected.size === 0 ? "*" : Array.from(selected).join(",");
      const created = await api.createWebhook({ name: name.trim(), url: url.trim(), secret: secret.trim() || undefined, events, enabled: true } as any);
      setWebhooks((w) => [created, ...w]); setShow(false); setName(""); setUrl(""); setSecret("");
    } catch (err: any) { toast.error(err.message || t("settings.createFailed")); } finally { setBusy(false); }
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("settings.webhooksTitle")} description={t("settings.webhooksDescription")} />
      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">{t("settings.outboundEventWebhooks")}</h2>
          <Button variant="ghost" onClick={() => { setName(""); setUrl(""); setSecret(""); setAll(true); setSelected(new Set()); setShow(true); }} className="flex items-center gap-1.5 px-3 py-1 text-xs">
            <Plus className="h-3 w-3" /> {t("settings.addWebhook")}
          </Button>
        </div>
        {webhooks.length === 0 ? (
          <div className="select-none rounded border border-dashed border-white/[0.06] py-4 text-center text-xs text-white/40">{t("settings.noWebhooks")}</div>
        ) : (
          <div className="space-y-3.5">
            {webhooks.map((w) => (
              <div key={w.id} className="flex flex-col justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm md:flex-row md:items-center">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-white/80">{w.name}</span>
                    {w.events === "*" ? (
                      <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono text-[9px] uppercase text-white/45">{t("settings.allEvents")}</span>
                    ) : (
                      String(w.events).split(",").filter(Boolean).map((key: string) => {
                        const def = defByKey.get(key.trim());
                        return (
                          <span key={key} title={def ? `${def.group} — ${def.title}: ${def.description}` : undefined} className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono text-[9px] text-white/45">
                            {key.trim()}
                          </span>
                        );
                      })
                    )}
                  </div>
                  <div className="select-all truncate font-mono text-xs text-white/45">{w.url}</div>
                  <div className="select-all font-mono text-[10px] text-zinc-500">{t("settings.secretLabel")} {w.secret}</div>
                </div>
                <div className="flex shrink-0 items-center gap-3 self-end md:self-auto">
                  <Toggle on={w.enabled} onChange={() => toggle(w)} />
                  <Button variant="danger" onClick={() => del(w.id)} className="flex items-center gap-1 px-2.5 py-1 text-xs"><Trash2 className="h-3 w-3" /> {t("settings.delete")}</Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {show && (
        <Modal title={t("settings.addWebhookEndpoint")} onClose={() => setShow(false)}>
          <form onSubmit={create} className="space-y-4">
            <Field label={t("settings.endpointName")}><input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="n8n automation" required autoFocus /></Field>
            <Field label={t("settings.endpointUrl")}><input className="input w-full font-mono text-xs" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-server.com/webhooks/octarq" required /></Field>
            <Field label={t("settings.signingSecretOptional")} hint={t("settings.signingSecretHint")}>
              <input className="input w-full font-mono text-xs" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={t("settings.signingSecretPlaceholder")} />
            </Field>
            <Field label={t("settings.eventSubscriptions")}>
              <div className="mt-1 space-y-2">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="radio" name="webhook-events-mode" checked={all} onChange={() => setAll(true)} />
                  <span>{t("settings.allEventsStar")}</span>
                </label>
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="radio" name="webhook-events-mode" checked={!all} onChange={() => setAll(false)} />
                  <span>{t("settings.customEvents")}</span>
                </label>
                {!all && (
                  <div className="max-h-64 space-y-3 overflow-y-auto rounded-lg border border-white/[0.06] bg-black/20 p-3">
                    {eventGroups === null ? (
                      <div className="py-2 text-center text-xs text-white/40">{t("settings.loadingEvents")}</div>
                    ) : (
                      eventGroups.map((g) => {
                        const allChecked = g.events.every((ev) => selected.has(ev.key));
                        return (
                          <div key={g.group} className="space-y-1.5">
                            <label className="flex cursor-pointer items-center gap-2 text-xs font-semibold text-white/70">
                              <input type="checkbox" checked={allChecked} onChange={(e) => toggleGroup(g, e.target.checked)} />
                              <span>{g.group}</span>
                            </label>
                            <div className="space-y-1.5 pl-6">
                              {g.events.map((ev) => (
                                <label key={ev.key} className="flex cursor-pointer items-start gap-2 text-xs text-zinc-300">
                                  <input type="checkbox" className="mt-0.5" checked={selected.has(ev.key)} onChange={(e) => toggleEvent(ev.key, e.target.checked)} />
                                  <span className="min-w-0">
                                    <span className="flex flex-wrap items-center gap-1.5">
                                      <span>{ev.title}</span>
                                      <span className="rounded border border-white/10 bg-white/5 px-1 py-px font-mono text-[9px] text-white/45">{ev.key}</span>
                                    </span>
                                    <span className="block text-[10px] text-white/40">{ev.description}</span>
                                  </span>
                                </label>
                              ))}
                            </div>
                          </div>
                        );
                      })
                    )}
                    {selected.size === 0 && <div className="text-[10px] text-amber-400/80">{t("settings.noEventsSelectedHint")}</div>}
                  </div>
                )}
              </div>
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setShow(false)}>{t("settings.cancel")}</Button>
              <Button type="submit" variant="primary" disabled={busy}>{busy ? t("settings.adding") : t("settings.add")}</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

export function NotificationChannels() {
  const { t } = useTranslation();
  const [channels, setChannels] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setChannels(await api.notificationChannels());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    load();
  }, []);

  async function remove(id: number) {
    if (!confirm(t("settings.confirmDeleteChannel"))) return;
    await api.deleteNotificationChannel(id);
    load();
  }

  async function test(id: number) {
    try {
      await api.testNotificationChannel(id);
      toast.success(t("settings.testAlertSent"));
    } catch (err: any) {
      toast.error(t("settings.testFailed", { msg: err.message }));
    }
  }

  async function toggleEnabled(c: any) {
    await api.updateNotificationChannel(c.id, { enabled: !c.enabled });
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("settings.alertsTitle")}
        description={t("settings.alertsDescription")}
        action={
          <Button variant="primary" onClick={() => setEditing({ type: "telegram", config: "{}" })}>
            {t("settings.addChannel")}
          </Button>
        }
      />
      <GlassCard className="p-6">

      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">{t("settings.loadingLower")}</div>
      ) : channels.length === 0 ? (
        <Empty>
          <Bell className="h-8 w-8 text-white/50 mb-1" />
          <div className="text-xs text-white/50">{t("settings.noChannels")}</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {channels.map((c) => {
            const channelTypeTone = c.type === "telegram" ? "cyan" : "violet";

            return (
              <div key={c.id} className="flex items-center gap-3 p-4 group">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-semibold text-sm text-white">{c.name}</span>
                    <Badge tone={channelTypeTone} className="uppercase tracking-wider text-[9px]">
                      {c.type}
                    </Badge>
                    {!c.enabled && <Badge tone="neutral">{t("settings.badgeDisabled")}</Badge>}
                  </div>
                  <div className="text-[11px] text-white/50 mt-1">{t("settings.added", { time: timeAgo(c.createdAt) })}</div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="subtle"
                    onClick={() => toggleEnabled(c)}
                    className="text-xs py-1 px-2.5"
                  >
                    {c.enabled ? t("settings.disable") : t("settings.enable")}
                  </Button>
                  <Button variant="outline" onClick={() => test(c.id)} className="text-xs py-1 px-2.5">
                    {t("settings.test")}
                  </Button>
                  <Button variant="ghost" onClick={() => setEditing(c)} className="text-xs py-1 px-2.5">
                    <Pencil className="h-3 w-3" />
                  </Button>
                  <Button
                    variant="danger"
                    onClick={() => remove(c.id)}
                    className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {editing && (
        <EditNotificationChannel
          channel={editing.id ? editing : null}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}
    </GlassCard>
    </div>
  );
}

function EditNotificationChannel({ channel, onClose, onSaved }: { channel: any; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(channel?.name || "");
  const [type, setType] = useState(channel?.type || "telegram");
  const [enabled, setEnabled] = useState(channel?.id ? channel.enabled : true);

  const initialCfg = channel?.id ? JSON.parse(channel.config) : {};
  const [botToken, setBotToken] = useState(initialCfg.botToken || "");
  const [chatId, setChatId] = useState(initialCfg.chatId || "");
  const [webhookUrl, setWebhookUrl] = useState(initialCfg.url || "");

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function save() {
    setBusy(true);
    setError("");
    let configStr = "{}";
    if (type === "telegram") {
      configStr = JSON.stringify({ botToken, chatId });
    } else if (type === "webhook") {
      configStr = JSON.stringify({ url: webhookUrl });
    }

    try {
      if (channel?.id) {
        await api.updateNotificationChannel(channel.id, {
          name,
          type,
          config: configStr,
          enabled,
        });
      } else {
        await api.createNotificationChannel({
          name,
          type,
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
    <Modal title={channel ? t("settings.editAlertChannel") : t("settings.createAlertChannel")} onClose={onClose}>
      <form onSubmit={(e) => { e.preventDefault(); save(); }} className="space-y-4">
        <Field label={t("settings.channelName")} hint={t("settings.channelNameHint")}>
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Dev Team Slack"
            required
            autoFocus
          />
        </Field>
        
        <Field label={t("settings.channelIntegrationType")}>
          <Select
            value={type}
            onValueChange={setType}
            options={[
              { value: "telegram", label: t("settings.optTelegram") },
              { value: "webhook", label: t("settings.optWebhook") },
            ]}
          />
        </Field>

        {type === "telegram" && (
          <>
            <Field label={t("settings.botAuthToken")} hint={t("settings.botAuthTokenHint")}>
              <input className="input w-full font-mono text-xs" value={botToken} onChange={(e) => setBotToken(e.target.value)} required />
            </Field>
            <Field label={t("settings.telegramChatId")} hint={t("settings.telegramChatIdHint")}>
              <input className="input w-full font-mono text-xs" value={chatId} onChange={(e) => setChatId(e.target.value)} required />
            </Field>
          </>
        )}

        {type === "webhook" && (
          <Field label={t("settings.customHttpTargetUrl")} hint={t("settings.customHttpTargetHint")}>
            <input className="input w-full font-mono text-xs" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} placeholder="https://my-webhook.com/alerts" required />
          </Field>
        )}

        {error && <div className="text-rose-400 text-xs font-semibold">{error}</div>}

        <div className="flex items-center gap-3 pt-2">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">{t("settings.channelEnabled")}</span>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/6">
          <Button type="button" variant="ghost" onClick={onClose}>{t("settings.cancel")}</Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? t("settings.savingDots") : t("settings.saveChannel")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

