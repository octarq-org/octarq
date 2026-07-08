import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember, LicenseStatus, Overview, PluginInfo } from "../../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign, Puzzle } from "lucide-react";
import { useTranslation } from "../../i18n";
import LLMProvidersSettings from "../LLMProviders";
import { useSettingsData, SavedBadge } from "./shared";

export function WebhooksSettings() {
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [show, setShow] = useState(false);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [all, setAll] = useState(true);
  const [evClick, setEvClick] = useState(false);
  const [evEmail, setEvEmail] = useState(false);
  const [busy, setBusy] = useState(false);

  function load() { api.webhooks().then(setWebhooks).catch(() => {}); }
  useEffect(load, []);

  async function del(id: number) { if (!confirm("Delete this webhook endpoint?")) return; await api.deleteWebhook(id); setWebhooks((w) => w.filter((h) => h.id !== id)); }
  async function toggle(h: any) { const u = await api.updateWebhook(h.id, { enabled: !h.enabled }); setWebhooks((w) => w.map((x) => x.id === h.id ? u : x)); }
  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    setBusy(true);
    try {
      let events = "*";
      if (!all) { const l: string[] = []; if (evClick) l.push("link.click"); if (evEmail) l.push("email.receive"); events = l.join(",") || "*"; }
      const created = await api.createWebhook({ name: name.trim(), url: url.trim(), secret: secret.trim() || undefined, events, enabled: true } as any);
      setWebhooks((w) => [created, ...w]); setShow(false); setName(""); setUrl(""); setSecret("");
    } catch (err: any) { alert(err.message || "create failed"); } finally { setBusy(false); }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="Webhooks" description="Send click and email events to your own systems in real time. Every request is signed so you can verify it came from octarq." />
      <GlassCard className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-bold text-white">Outbound Event Webhooks</h2>
          <Button variant="ghost" onClick={() => { setName(""); setUrl(""); setSecret(""); setAll(true); setEvClick(false); setEvEmail(false); setShow(true); }} className="flex items-center gap-1.5 px-3 py-1 text-xs">
            <Plus className="h-3 w-3" /> Add Webhook
          </Button>
        </div>
        {webhooks.length === 0 ? (
          <div className="select-none rounded border border-dashed border-white/[0.06] py-4 text-center text-xs text-white/40">No outbound webhooks configured.</div>
        ) : (
          <div className="space-y-3.5">
            {webhooks.map((w) => (
              <div key={w.id} className="flex flex-col justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm md:flex-row md:items-center">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold text-white/80">{w.name}</span>
                    <span className="rounded border border-white/10 bg-white/5 px-1.5 py-0.5 font-mono text-[9px] uppercase text-white/45">{w.events === "*" ? "all events" : w.events}</span>
                  </div>
                  <div className="select-all truncate font-mono text-xs text-white/45">{w.url}</div>
                  <div className="select-all font-mono text-[10px] text-zinc-500">Secret: {w.secret}</div>
                </div>
                <div className="flex shrink-0 items-center gap-3 self-end md:self-auto">
                  <Toggle on={w.enabled} onChange={() => toggle(w)} />
                  <Button variant="danger" onClick={() => del(w.id)} className="flex items-center gap-1 px-2.5 py-1 text-xs"><Trash2 className="h-3 w-3" /> Delete</Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </GlassCard>

      {show && (
        <Modal title="Add Webhook Endpoint" onClose={() => setShow(false)}>
          <form onSubmit={create} className="space-y-4">
            <Field label="Endpoint Name"><input className="input w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="n8n automation" required autoFocus /></Field>
            <Field label="Endpoint URL"><input className="input w-full font-mono text-xs" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://your-server.com/webhooks/octarq" required /></Field>
            <Field label="Signing Secret (Optional)" hint="Signs the payload in X-Octarq-Signature. Leave empty to auto-generate.">
              <input className="input w-full font-mono text-xs" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder="Custom signing secret" />
            </Field>
            <Field label="Event Subscriptions">
              <div className="mt-1 space-y-2">
                <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                  <input type="checkbox" checked={all} onChange={(e) => { setAll(e.target.checked); if (e.target.checked) { setEvClick(false); setEvEmail(false); } }} />
                  <span>All Events (*)</span>
                </label>
                {!all && (
                  <div className="space-y-2 pl-6">
                    <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300"><input type="checkbox" checked={evClick} onChange={(e) => setEvClick(e.target.checked)} /> <span>link.click</span></label>
                    <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300"><input type="checkbox" checked={evEmail} onChange={(e) => setEvEmail(e.target.checked)} /> <span>email.receive</span></label>
                  </div>
                )}
              </div>
            </Field>
            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setShow(false)}>Cancel</Button>
              <Button type="submit" variant="primary" disabled={busy}>{busy ? "Adding…" : "Add"}</Button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

export function NotificationChannels() {
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
    if (!confirm("Delete this notification channel?")) return;
    await api.deleteNotificationChannel(id);
    load();
  }

  async function test(id: number) {
    try {
      await api.testNotificationChannel(id);
      alert("Test alert sent successfully!");
    } catch (err: any) {
      alert("Test failed: " + err.message);
    }
  }

  async function toggleEnabled(c: any) {
    await api.updateNotificationChannel(c.id, { enabled: !c.enabled });
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Alerts"
        description="Get notified in Telegram, Slack, or your own webhook when important events happen."
        action={
          <Button variant="primary" onClick={() => setEditing({ type: "telegram", config: "{}" })}>
            + Add Channel
          </Button>
        }
      />
      <GlassCard className="p-6">

      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : channels.length === 0 ? (
        <Empty>
          <Bell className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No notification channels configured.</div>
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
                    {!c.enabled && <Badge tone="neutral">disabled</Badge>}
                  </div>
                  <div className="text-[11px] text-white/35 mt-1">Added {timeAgo(c.createdAt)}</div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <Button
                    variant="subtle"
                    onClick={() => toggleEnabled(c)}
                    className="text-xs py-1 px-2.5"
                  >
                    {c.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button variant="outline" onClick={() => test(c.id)} className="text-xs py-1 px-2.5">
                    Test
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
      setError(err.message || "Failed to save");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={channel ? "Edit Alert Channel" : "Create Alert Channel"} onClose={onClose}>
      <form onSubmit={(e) => { e.preventDefault(); save(); }} className="space-y-4">
        <Field label="Channel Name" hint="A memorable identifier for this trigger">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Dev Team Slack"
            required
            autoFocus
          />
        </Field>
        
        <Field label="Channel Integration Type">
          <select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="telegram">Telegram Bot webhook</option>
            <option value="webhook">Custom HTTP POST Webhook</option>
          </select>
        </Field>

        {type === "telegram" && (
          <>
            <Field label="Bot Authentication Token" hint="Token issued by Telegram @BotFather">
              <input className="input w-full font-mono text-xs" value={botToken} onChange={(e) => setBotToken(e.target.value)} required />
            </Field>
            <Field label="Telegram Chat ID" hint="Channel group ID or user chat ID to forward alerts">
              <input className="input w-full font-mono text-xs" value={chatId} onChange={(e) => setChatId(e.target.value)} required />
            </Field>
          </>
        )}

        {type === "webhook" && (
          <Field label="Custom HTTP Target URL" hint="Receives JSON payload POST: { text: 'string' }">
            <input className="input w-full font-mono text-xs" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} placeholder="https://my-webhook.com/alerts" required />
          </Field>
        )}

        {error && <div className="text-rose-400 text-xs font-semibold">{error}</div>}

        <div className="flex items-center gap-3 pt-2">
          <Toggle on={enabled} onChange={setEnabled} />
          <span className="text-sm text-white/60 select-none">Channel Enabled</span>
        </div>

        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name}>
            {busy ? "Saving..." : "Save Channel"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

