import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings as SettingsData, OrgMember } from "../api";
import { Empty, Field, Modal, Toggle, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { Settings as SettingsIcon, Cloud, Mail, Bell, Users, Trash2, Pencil, ShieldAlert, KeyRound, BellRing, Webhook, Plus, Send, AlertTriangle, CreditCard, Sparkles, Shield, DollarSign } from "lucide-react";

export default function SettingsPage() {
  return (
    <ScreenWrap>
      <Routes>
        <Route path="/" element={<Navigate to="/settings/general" replace />} />
        <Route path="/general" element={<GeneralSettings />} />
        <Route path="/billing" element={<BillingPlanDemo />} />
        <Route path="/providers" element={<ProviderAccounts />} />
        <Route path="/smtp" element={<SMTPSenders />} />
        <Route path="/notifications" element={<NotificationChannels />} />
        <Route path="/members" element={<OrgMembersManager />} />
      </Routes>
    </ScreenWrap>
  );
}

function GeneralSettings() {
  const [s, setS] = useState<SettingsData | null>(null);
  const [workspaceName, setWorkspaceName] = useState("");
  const [workspaceSaved, setWorkspaceSaved] = useState(false);
  const [workspaceRenameBusy, setWorkspaceRenameBusy] = useState(false);
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [reservedMailboxes, setReservedMailboxes] = useState("");
  const [cfToken, setCfToken] = useState("");
  const [inboundToken, setInboundToken] = useState("");
  const [catchAll, setCatchAll] = useState(false);
  const [autoWrapLinks, setAutoWrapLinks] = useState(false);
  const [telegramBot, setTelegramBot] = useState("");
  const [telegramChat, setTelegramChat] = useState("");
  const [googleClientId, setGoogleClientId] = useState("");
  const [googleClientSecret, setGoogleClientSecret] = useState("");
  const [githubClientId, setGithubClientId] = useState("");
  const [githubClientSecret, setGithubClientSecret] = useState("");
  const [dataRetentionDays, setDataRetentionDays] = useState(90);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  // Outbound Webhooks state
  const [webhooks, setWebhooks] = useState<any[]>([]);
  const [showAddWebhookModal, setShowAddWebhookModal] = useState(false);
  const [newHookName, setNewHookName] = useState("");
  const [newHookURL, setNewHookURL] = useState("");
  const [newHookSecret, setNewHookSecret] = useState("");
  const [newHookEventsAll, setNewHookEventsAll] = useState(true);
  const [newHookEventsClick, setNewHookEventsClick] = useState(false);
  const [newHookEventsEmail, setNewHookEventsEmail] = useState(false);
  const [webhooksBusy, setWebhooksBusy] = useState(false);

  async function load() {
    try {
      const me = await api.me();
      const orgs = await api.orgs();
      const currentOrg = orgs.find(o => o.id === me.orgId);
      if (currentOrg) {
        setWorkspaceName(currentOrg.name);
      }
    } catch (e) {
      console.error("Failed to load workspace name:", e);
    }

    try {
      const wList = await api.webhooks();
      setWebhooks(wList);
    } catch (e) {
      console.error("Failed to load webhooks:", e);
    }

    const v = await api.settings();
    setS(v);
    setReservedSlugs(v.reservedSlugs);
    setReservedMailboxes(v.reservedMailboxes);
    setInboundToken(v.inboundToken || "");
    setCatchAll(v.catchAll || false);
    setAutoWrapLinks(v.autoWrapLinks || false);
    setTelegramBot(v.telegramBotToken || "");
    setTelegramChat(v.telegramChatId || "");
    setGoogleClientId(v.googleClientId || "");
    setGithubClientId(v.githubClientId || "");
    setDataRetentionDays(v.dataRetentionDays ?? 90);
  }
  
  useEffect(() => {
    load();
  }, []);

  async function handleRenameWorkspace(e: React.FormEvent) {
    e.preventDefault();
    if (!workspaceName.trim()) return;
    setWorkspaceRenameBusy(true);
    setWorkspaceSaved(false);
    try {
      await api.updateOrg({ name: workspaceName });
      setWorkspaceSaved(true);
      setTimeout(() => setWorkspaceSaved(false), 2000);
      // Reload page to refresh initials in sidebar rail
      setTimeout(() => window.location.reload(), 1000);
    } catch (err: any) {
      alert(err.message || "Failed to rename workspace");
    } finally {
      setWorkspaceRenameBusy(false);
    }
  }

  async function save() {
    setBusy(true);
    setSaved(false);
    try {
      const payload: any = {
        reservedSlugs, reservedMailboxes,
        inboundToken, catchAll,
        telegramBotToken: telegramBot, telegramChatId: telegramChat,
        googleClientId: googleClientId.trim(),
        githubClientId: githubClientId.trim(),
        dataRetentionDays,
        autoWrapLinks,
      };
      if (cfToken.trim()) payload.cloudflareToken = cfToken.trim();
      if (googleClientSecret.trim()) payload.googleClientSecret = googleClientSecret.trim();
      if (githubClientSecret.trim()) payload.githubClientSecret = githubClientSecret.trim();
      const v = await api.updateSettings(payload);
      setS(v);
      setCfToken("");
      setGoogleClientSecret("");
      setGithubClientSecret("");
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } finally {
      setBusy(false);
    }
  }

  async function handleDeleteWebhook(id: number) {
    if (!confirm("Are you sure you want to delete this Webhook endpoint?")) return;
    try {
      await api.deleteWebhook(id);
      setWebhooks(webhooks.filter(h => h.id !== id));
    } catch (err: any) {
      alert(err.message || "Failed to delete Webhook");
    }
  }

  async function handleToggleWebhook(hook: any) {
    try {
      const updated = await api.updateWebhook(hook.id, { enabled: !hook.enabled });
      setWebhooks(webhooks.map(h => h.id === hook.id ? updated : h));
    } catch (err: any) {
      alert(err.message || "Failed to update Webhook");
    }
  }

  async function handleCreateWebhook(e: React.FormEvent) {
    e.preventDefault();
    if (!newHookName.trim() || !newHookURL.trim()) return;
    setWebhooksBusy(true);
    try {
      let events = "*";
      if (!newHookEventsAll) {
        const evList: string[] = [];
        if (newHookEventsClick) evList.push("link.click");
        if (newHookEventsEmail) evList.push("email.receive");
        events = evList.join(",") || "*";
      }
      const created = await api.createWebhook({
        name: newHookName.trim(),
        url: newHookURL.trim(),
        secret: newHookSecret.trim() || undefined,
        events,
        enabled: true,
      });
      setWebhooks([created, ...webhooks]);
      setShowAddWebhookModal(false);
      setNewHookName("");
      setNewHookURL("");
      setNewHookSecret("");
    } catch (err: any) {
      alert(err.message || "Failed to create Webhook");
    } finally {
      setWebhooksBusy(false);
    }
  }

  if (!s) return <div className="text-white/40 text-sm">loading…</div>;

  return (
    <div className="space-y-6">
      <PageHeader
        title="General Settings"
        description="Configure your workspace details, short-link settings, and API integrations"
      />

      {/* Workspace Identity Card */}
      <GlassCard className="p-6 space-y-4">
        <div className="flex justify-between items-center">
          <div>
            <h2 className="text-base font-bold text-white mb-1">Workspace Profile</h2>
            <p className="text-xs text-white/50">Manage the identity and name of this workspace.</p>
          </div>
          {workspaceSaved && <Badge tone="green">✓ Name Updated</Badge>}
        </div>

        <form onSubmit={handleRenameWorkspace} className="max-w-md">
          <Field label="Workspace Name" hint="This name is shown in the workspace switcher and header.">
            <div className="flex gap-2">
              <input
                className="input flex-1 text-sm"
                value={workspaceName}
                onChange={(e) => setWorkspaceName(e.target.value)}
                placeholder="e.g. Acme Production"
                required
              />
              <Button type="submit" variant="primary" disabled={workspaceRenameBusy || !workspaceName.trim()} className="shrink-0">
                {workspaceRenameBusy ? "Updating..." : "Update Name"}
              </Button>
            </div>
          </Field>
        </form>
      </GlassCard>

      {/* General Runtime Config Settings Card */}
      <GlassCard className="p-6 space-y-6">
        <div className="flex justify-between items-center">
          <div>
            <h2 className="text-base font-bold text-white mb-1">General Settings</h2>
            <p className="text-xs text-white/50">Runtime configuration parameters for this workspace.</p>
          </div>
          {saved && <Badge tone="green">✓ Config Saved</Badge>}
        </div>

        <div className="space-y-6">
          <Field
            label="Reserved Short Link Slugs"
            hint={`Slugs that users cannot register. Built-in defaults always locked: ${s.builtinReserved.join(", ")}.`}
          >
            <textarea
              className="input font-mono text-xs w-full"
              rows={3}
              value={reservedSlugs}
              onChange={(e) => setReservedSlugs(e.target.value)}
              placeholder="pricing&#10;login&#10;about"
            />
          </Field>
          
          <Field
            label="Reserved Inbound Mailbox Prefixes"
            hint="Prefixes that catch-all routing will not auto-provision (e.g. admin, postmaster)."
          >
            <textarea
              className="input font-mono text-xs w-full"
              rows={2}
              value={reservedMailboxes}
              onChange={(e) => setReservedMailboxes(e.target.value)}
              placeholder="admin&#10;postmaster"
            />
          </Field>
          
          <Field
            label="Global Cloudflare API Token"
            hint={
              s.cloudflareTokenSet
                ? "Global token is configured. Enter a new token to overwrite."
                : "Fallback token used by sync if individual domains don't provide dedicated keys. Zone:Read + DNS:Edit."
            }
          >
            <div className="flex gap-2">
              <input
                type="password"
                className="input w-full font-mono text-xs"
                value={cfToken}
                onChange={(e) => setCfToken(e.target.value)}
                placeholder={s.cloudflareTokenSet ? "•••••••• (Token set)" : "Cloudflare API token"}
              />
              {s.cloudflareTokenSet && (
                <Button
                  variant="danger"
                  onClick={async () => {
                    if (confirm("Clear stored token?")) {
                      await api.updateSettings({ cloudflareToken: "" });
                      load();
                    }
                  }}
                  className="py-1 px-3 text-xs bg-rose-500/10 hover:bg-rose-500/25 border-0"
                >
                  Clear
                </Button>
              )}
            </div>
          </Field>
          
          <div className="border-t border-white/[0.06] pt-6 space-y-4">
            <h3 className="text-sm font-semibold text-white/80">Mail Inbound Webhooks</h3>
            <div className="space-y-4">
              <Field
                label="Webhook Inbound Token"
                hint="Shared API secret validated in X-Led-Token header for Cloudflare Email Worker webhook trigger."
              >
                <input
                  className="input w-full font-mono text-xs"
                  value={inboundToken}
                  onChange={(e) => setInboundToken(e.target.value)}
                  placeholder="secret-token-value"
                />
              </Field>
              
              <div className="flex items-center gap-3 pt-2">
                <Toggle on={catchAll} onChange={setCatchAll} />
                <div>
                  <span className="text-xs font-semibold text-white/70 select-none block">Enable Catch-All routing</span>
                  <span className="text-[10px] text-white/40 select-none">Automatically provision local inbox addresses when a message arrives for an unknown managed alias.</span>
                </div>
              </div>

              <div className="flex items-center gap-3 pt-2 border-t border-white/[0.04]">
                <Toggle on={autoWrapLinks} onChange={setAutoWrapLinks} />
                <div>
                  <span className="text-xs font-semibold text-white/70 select-none block">Auto Wrap Outbound Links</span>
                  <span className="text-[10px] text-white/40 select-none">When sending outbound emails, automatically detect external URLs and wrap them as short links for click analytics. Checkbox is shown on compose mail modal.</span>
                </div>
              </div>
            </div>
          </div>

          <div className="border-t border-white/[0.06] pt-6 space-y-4">
            <div className="flex justify-between items-center">
              <h3 className="text-sm font-semibold text-white/80">Outbound Event Webhooks</h3>
              <Button
                variant="ghost"
                onClick={() => {
                  setNewHookName("");
                  setNewHookURL("");
                  setNewHookSecret("");
                  setNewHookEventsAll(true);
                  setNewHookEventsClick(false);
                  setNewHookEventsEmail(false);
                  setShowAddWebhookModal(true);
                }}
                className="py-1 px-3 text-xs bg-purple-500/10 hover:bg-purple-500/25 border-0 flex items-center gap-1.5"
              >
                <Plus className="h-3 w-3" /> Add Webhook
              </Button>
            </div>
            
            <p className="text-[10px] text-white/40">
              Deliver real-time click and email events to external systems (Zapier, n8n, custom APIs). Every dispatch payload is signed using HMAC-SHA256 for secure request validation.
            </p>

            {webhooks.length === 0 ? (
              <div className="py-4 text-center border border-dashed border-white/[0.06] rounded text-white/40 text-xs select-none">
                No outbound webhooks configured.
              </div>
            ) : (
              <div className="space-y-3.5">
                {webhooks.map((w) => (
                  <div key={w.id} className="p-3 bg-white/[0.02] border border-white/[0.06] rounded-lg flex flex-col md:flex-row md:items-center justify-between gap-3 text-sm">
                    <div className="space-y-1 flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-white/80">{w.name}</span>
                        <span className="text-[9px] font-mono uppercase bg-white/5 border border-white/10 px-1.5 py-0.5 rounded text-white/45">
                          {w.events === "*" ? "all events" : w.events}
                        </span>
                      </div>
                      <div className="font-mono text-xs text-white/45 truncate select-all">{w.url}</div>
                      <div className="font-mono text-[10px] text-zinc-500 select-all">Secret key: {w.secret}</div>
                    </div>
                    <div className="flex items-center gap-3 self-end md:self-auto shrink-0">
                      <Toggle on={w.enabled} onChange={() => handleToggleWebhook(w)} />
                      <Button
                        variant="danger"
                        onClick={() => handleDeleteWebhook(w.id)}
                        className="py-1 px-2.5 text-xs bg-rose-500/10 hover:bg-rose-500/20 hover:text-rose-300 border-0 flex items-center gap-1"
                      >
                        <Trash2 className="h-3 w-3" /> Delete
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="border-t border-white/[0.06] pt-6">
            <h3 className="text-sm font-semibold text-white/80 mb-2">Data Retention & Pruning</h3>
            <div className="space-y-4">
              <Field label="Click Event Logs Expiry (Days)" hint="Link clicks data older than this limit will be auto-deleted. Set 0 to persist forever.">
                <input
                  type="number"
                  min={0}
                  className="input w-32 font-mono text-sm"
                  value={dataRetentionDays}
                  onChange={(e) => setDataRetentionDays(Number(e.target.value))}
                />
              </Field>
              <p className="text-[10px] text-white/40">
                IP addresses of clickers are stored anonymized (masked subnet). This setting controls how long click statistics charts persist.
              </p>
            </div>
          </div>

          <div className="border-t border-white/[0.06] pt-6 space-y-4">
            <h3 className="text-sm font-semibold text-white/80">Telegram Alerts</h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Field label="Telegram Bot Token" hint="Token issued by @BotFather">
                <input
                  className="input w-full font-mono text-xs"
                  value={telegramBot}
                  onChange={(e) => setTelegramBot(e.target.value)}
                  placeholder="123456789:ABCdef..."
                />
              </Field>
              <Field label="Target Chat ID" hint="Individual or Group chat ID">
                <input
                  className="input w-full font-mono text-xs"
                  value={telegramChat}
                  onChange={(e) => setTelegramChat(e.target.value)}
                  placeholder="e.g. -100123456"
                />
              </Field>
            </div>
          </div>

          <div className="border-t border-white/[0.06] pt-6 space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-white/80">Single Sign-On (SSO / OAuth)</h3>
              <p className="text-[10px] text-white/40 mt-0.5">Secrets are encrypted. Make sure server callback URLs matches LED base url.</p>
            </div>
            
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Google */}
              <div className="rounded-xl border border-white/[0.05] bg-black/20 p-4 space-y-3">
                <p className="text-xs font-bold text-white/85 flex items-center gap-1.5">
                  <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />
                  Google Sign-In
                </p>
                <Field label="Google Client ID">
                  <input
                    className="input w-full text-xs"
                    value={googleClientId}
                    onChange={(e) => setGoogleClientId(e.target.value)}
                    placeholder="*.apps.googleusercontent.com"
                  />
                </Field>
                <Field label="Google Client Secret">
                  <div className="flex gap-2">
                    <input
                      className="input w-full text-xs font-mono"
                      type="password"
                      value={googleClientSecret}
                      onChange={(e) => setGoogleClientSecret(e.target.value)}
                      placeholder={s.googleClientSecretSet ? "•••••••• (Set)" : "Secret value"}
                    />
                    {s.googleClientSecretSet && (
                      <Button
                        variant="danger"
                        onClick={async () => {
                          if (confirm("Clear Google secret?")) {
                            await api.updateSettings({ googleClientSecret: "" });
                            load();
                          }
                        }}
                        className="py-1 px-2.5 text-xs bg-rose-500/10 hover:bg-rose-500/25 border-0"
                      >
                        Clear
                      </Button>
                    )}
                  </div>
                </Field>
              </div>
              
              {/* GitHub */}
              <div className="rounded-xl border border-white/[0.05] bg-black/20 p-4 space-y-3">
                <p className="text-xs font-bold text-white/85 flex items-center gap-1.5">
                  <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />
                  GitHub Integration
                </p>
                <Field label="GitHub Client ID">
                  <input
                    className="input w-full text-xs"
                    value={githubClientId}
                    onChange={(e) => setGithubClientId(e.target.value)}
                    placeholder="Ov23li..."
                  />
                </Field>
                <Field label="GitHub Client Secret">
                  <div className="flex gap-2">
                    <input
                      className="input w-full text-xs font-mono"
                      type="password"
                      value={githubClientSecret}
                      onChange={(e) => setGithubClientSecret(e.target.value)}
                      placeholder={s.githubClientSecretSet ? "•••••••• (Set)" : "Secret value"}
                    />
                    {s.githubClientSecretSet && (
                      <Button
                        variant="danger"
                        onClick={async () => {
                          if (confirm("Clear GitHub secret?")) {
                            await api.updateSettings({ githubClientSecret: "" });
                            load();
                          }
                        }}
                        className="py-1 px-2.5 text-xs bg-rose-500/10 hover:bg-rose-500/25 border-0"
                      >
                        Clear
                      </Button>
                    )}
                  </div>
                </Field>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-3 border-t border-white/[0.06] pt-6">
            <Button variant="primary" onClick={save} disabled={busy}>
              {busy ? "Saving..." : "Save Config Settings"}
            </Button>
          </div>
        </div>
        {showAddWebhookModal && (
          <Modal title="Add Webhook Endpoint" onClose={() => setShowAddWebhookModal(false)}>
            <form onSubmit={handleCreateWebhook} className="space-y-4">
              <Field label="Endpoint Name">
                <input
                  className="input w-full"
                  value={newHookName}
                  onChange={(e) => setNewHookName(e.target.value)}
                  placeholder="e.g. n8n automation"
                  required
                  autoFocus
                />
              </Field>
              
              <Field label="Endpoint URL">
                <input
                  className="input w-full font-mono text-xs"
                  value={newHookURL}
                  onChange={(e) => setNewHookURL(e.target.value)}
                  placeholder="https://your-server.com/webhooks/led"
                  required
                />
              </Field>
              
              <Field label="Signing Secret (Optional)" hint="Used to sign webhook payload in X-Led-Signature header. Leave empty to auto-generate.">
                <input
                  type="text"
                  className="input w-full font-mono text-xs"
                  value={newHookSecret}
                  onChange={(e) => setNewHookSecret(e.target.value)}
                  placeholder="Custom signing secret key"
                />
              </Field>

              <Field label="Event Subscriptions">
                <div className="space-y-2 mt-1">
                  <label className="flex items-center gap-2 cursor-pointer text-xs text-zinc-300">
                    <input
                      type="checkbox"
                      checked={newHookEventsAll}
                      onChange={(e) => {
                        setNewHookEventsAll(e.target.checked);
                        if (e.target.checked) {
                          setNewHookEventsClick(false);
                          setNewHookEventsEmail(false);
                        }
                      }}
                      className="rounded border-zinc-700 bg-zinc-900/50 text-purple-600 focus:ring-purple-500"
                    />
                    <span>All Events (*)</span>
                  </label>

                  {!newHookEventsAll && (
                    <div className="pl-6 space-y-2">
                      <label className="flex items-center gap-2 cursor-pointer text-xs text-zinc-300">
                        <input
                          type="checkbox"
                          checked={newHookEventsClick}
                          onChange={(e) => setNewHookEventsClick(e.target.checked)}
                          className="rounded border-zinc-700 bg-zinc-900/50 text-purple-600 focus:ring-purple-500"
                        />
                        <span>Shortlink clicks (link.click)</span>
                      </label>

                      <label className="flex items-center gap-2 cursor-pointer text-xs text-zinc-300">
                        <input
                          type="checkbox"
                          checked={newHookEventsEmail}
                          onChange={(e) => setNewHookEventsEmail(e.target.checked)}
                          className="rounded border-zinc-700 bg-zinc-900/50 text-purple-600 focus:ring-purple-500"
                        />
                        <span>Inbound email received (email.receive)</span>
                      </label>
                    </div>
                  )}
                </div>
              </Field>

              <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
                <Button type="button" variant="ghost" onClick={() => setShowAddWebhookModal(false)}>
                  Cancel
                </Button>
                <Button type="submit" variant="primary" disabled={webhooksBusy || !newHookName.trim() || !newHookURL.trim()}>
                  {webhooksBusy ? "Creating..." : "Create Webhook"}
                </Button>
              </div>
            </form>
          </Modal>
        )}
      </GlassCard>
    </div>
  );
}

function NotificationChannels() {
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
        title="Alert Hooks & Channels"
        description="Integrate chat systems and webhooks to receive real-time alerts when server/email events fire"
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

function OrgMembersManager() {
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  async function load() {
    setLoading(true);
    try {
      setMembers(await api.orgMembers());
    } catch (e: any) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault();
    if (!email) return;
    setBusy(true);
    setErr("");
    try {
      await api.addOrgMember({ email, role });
      setEmail("");
      setRole("member");
      load();
    } catch (e: any) {
      setErr(e.message || "Failed to add member");
    } finally {
      setBusy(false);
    }
  }

  async function handleRemove(userId: number) {
    if (!confirm("Remove this member from the workspace? They will lose access instantly.")) return;
    try {
      await api.deleteOrgMember(userId);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove member");
    }
  }

  const getRoleTone = (r: string) => {
    if (r === "owner") return "green";
    if (r === "admin") return "indigo";
    return "neutral";
  };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Workspace Members"
        description="Invite colleagues, assign administrative roles, and manage permissions within this workspace"
      />
      <GlassCard className="p-6 space-y-6">

      <form onSubmit={handleAdd} className="bg-black/25 p-4 rounded-xl border border-white/[0.05] flex flex-wrap sm:flex-nowrap gap-4 items-end">
        <div className="flex-1 min-w-[200px]">
          <label className="label text-xs">Invite Colleague by Email</label>
          <input
            className="input w-full text-sm mt-1"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
            required
          />
        </div>
        <div className="w-32">
          <label className="label text-xs">Access Role</label>
          <select className="input w-full text-xs mt-1" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </select>
        </div>
        <Button variant="primary" className="py-2 text-xs shrink-0" disabled={busy || !email}>
          {busy ? "Inviting..." : "Invite Member"}
        </Button>
      </form>
      {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}

      {loading ? (
        <div className="text-white/40 text-sm py-4 text-center">loading members list…</div>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {members.map((m) => (
            <div key={m.userId} className="flex justify-between items-center p-4">
              <div className="flex items-center gap-2.5">
                <span className="font-semibold text-sm text-white">{m.email}</span>
                <Badge tone={getRoleTone(m.role)} className="capitalize text-[10px] tracking-wide font-semibold px-2">
                  {m.role}
                </Badge>
              </div>
              <Button
                variant="danger"
                onClick={() => handleRemove(m.userId)}
                className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
              >
                Remove
              </Button>
            </div>
          ))}
        </div>
      )}
    </GlassCard>
    </div>
  );
}

function ProviderAccounts() {
  const [accounts, setAccounts] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setAccounts(await api.providerAccounts());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function remove(id: number) {
    if (!confirm("Remove this provider account? Managed domains using these credentials will fail sync.")) return;
    try {
      await api.deleteProviderAccount(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="DNS Providers"
        description="Configure API connections for Cloudflare and DNSPod to sync and verify domains"
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            + Add Provider
          </Button>
        }
      />
      <GlassCard className="p-6">
      
      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : accounts.length === 0 ? (
        <Empty>
          <Cloud className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No external provider connections configured yet.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {accounts.map(a => (
            <div key={a.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-semibold text-sm text-white">{a.name}</div>
                <div className="text-xs text-white/40 mt-1">
                  <Badge tone={a.type === "cloudflare" ? "indigo" : "cyan"} className="uppercase tracking-wider text-[9px]">
                    {a.type}
                  </Badge>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(a)} className="text-xs py-1 px-2.5">
                  Edit Key
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(a.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      {(creating || editing) && (
        <ProviderAccountModal
          account={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={() => { setCreating(false); setEditing(null); load(); }}
        />
      )}
    </GlassCard>
    </div>
  );
}

function ProviderAccountModal({ account, onClose, onSaved }: { account: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(account?.name || "");
  const [type, setType] = useState(account?.type || "cloudflare");
  const [config, setConfig] = useState<string>("");
  const [types, setTypes] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.dnsProviders().then(setTypes).catch(() => setTypes(["cloudflare", "dnspod"]));
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setErr("");
    try {
      let cfgObj: any = {};
      if (config.trim()) {
        try { cfgObj = JSON.parse(config.trim()); }
        catch { cfgObj = type === "cloudflare" ? { apiToken: config.trim() } : { token: config.trim() }; }
      }
      if (account) {
        await api.updateProviderAccount(account.id, { name, type, config: cfgObj });
      } else {
        await api.createProviderAccount({ name, type, config: cfgObj });
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message || "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={account ? "Edit Provider Account" : "Register Provider Account"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Provider Label Name">
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Acme Production DNS" required autoFocus />
        </Field>
        {!account && (
          <Field label="DNS Provider Type">
            <select className="input w-full text-sm" value={type} onChange={e => setType(e.target.value)}>
              {types.map(t => <option key={t} value={t} className="capitalize">{t}</option>)}
            </select>
          </Field>
        )}
        <Field label="API Keys / Credentials" hint={account ? "Leave blank to keep existing keys. Cloudflare takes Token. DNSPod takes 'ID,Token'." : "For Cloudflare, paste API Token. For DNSPod, paste 'ID,Token' format."}>
          <input className="input w-full font-mono text-xs" type="password" value={config} onChange={e => setConfig(e.target.value)} placeholder={account ? "••••••••" : "API Token / Key string..."} required={!account} />
        </Field>
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? "Saving..." : "Save Connection"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function SMTPSenders() {
  const [senders, setSenders] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<any | null>(null);

  async function load() {
    setLoading(true);
    try {
      setSenders(await api.smtpSenders());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => { load(); }, []);

  async function remove(id: number) {
    if (!confirm("Remove this SMTP sender config? Outbound mail relying on it will fail to send.")) return;
    try {
      await api.deleteSMTPSender(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="SMTP Senders"
        description="Configure external SMTP gateway relays used to send outgoing emails from mailboxes"
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            + Add SMTP
          </Button>
        }
      />
      <GlassCard className="p-6">

      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : senders.length === 0 ? (
        <Empty>
          <Send className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No SMTP outgoing senders configured yet.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {senders.map(s => (
            <div key={s.id} className="flex items-center justify-between p-4 group">
              <div>
                <div className="font-semibold text-sm text-white">{s.name}</div>
                <div className="text-xs text-white/40 mt-1 font-mono">
                  {s.fromEmail} via {s.host}:{s.port}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="subtle" onClick={() => setEditing(s)} className="text-xs py-1 px-2.5">
                  Edit SMTP
                </Button>
                <Button
                  variant="danger"
                  onClick={() => remove(s.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
      {(creating || editing) && (
        <SMTPSenderModal
          sender={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={() => { setCreating(false); setEditing(null); load(); }}
        />
      )}
    </GlassCard>
    </div>
  );
}

function SMTPSenderModal({ sender, onClose, onSaved }: { sender: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(sender?.name || "");
  const [host, setHost] = useState(sender?.host || "");
  const [port, setPort] = useState(sender?.port?.toString() || "");
  const [user, setUser] = useState(sender?.user || "");
  const [pass, setPass] = useState("");
  const [fromEmail, setFromEmail] = useState(sender?.fromEmail || "");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true); setErr("");
    try {
      const payload = { name, host, port: parseInt(port, 10), user, pass, fromEmail };
      if (sender) {
        await api.updateSMTPSender(sender.id, payload);
      } else {
        await api.createSMTPSender(payload);
      }
      onSaved();
    } catch (e: any) {
      setErr(e.message || "Failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title={sender ? "Modify SMTP Relay" : "Configure SMTP Relay"} onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Sender Connection Name">
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Corporate SMTP" required autoFocus />
        </Field>
        
        <div className="flex gap-4">
          <div className="flex-[3]">
            <Field label="SMTP Host Server address">
              <input className="input w-full font-mono text-xs" value={host} onChange={e => setHost(e.target.value)} placeholder="smtp.mailgun.org" required />
            </Field>
          </div>
          <div className="flex-1">
            <Field label="SMTP Port">
              <input type="number" className="input w-full font-mono text-xs" value={port} onChange={e => setPort(e.target.value)} placeholder="587" required />
            </Field>
          </div>
        </div>

        <Field label="SMTP Connection Username">
          <input className="input w-full font-mono text-xs" value={user} onChange={e => setUser(e.target.value)} placeholder="e.g. postmaster@domain.com" required />
        </Field>
        
        <Field label="SMTP Connection Password" hint={sender ? "Leave blank to preserve current secure key." : ""}>
          <input type="password" className="input w-full font-mono text-xs" value={pass} onChange={e => setPass(e.target.value)} placeholder="••••••••" required={!sender} />
        </Field>
        
        <Field label="Default From Address" hint="Outgoing address used if none specified.">
          <input className="input w-full font-mono text-xs" value={fromEmail} onChange={e => setFromEmail(e.target.value)} placeholder="noreply@domain.com" required />
        </Field>
        
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim() || !host.trim() || !port || !user.trim()}>
            {busy ? "Saving..." : "Save Relay"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function BillingPlanDemo() {
  const plans = [
    {
      name: "Starter",
      price: "$0",
      period: "forever",
      description: "Ideal for personal sites & hobby projects.",
      features: [
        "Up to 3 Managed Domains",
        "1,000 Short Links / month",
        "1 Workspace Member",
        "Basic Click Analytics",
        "Community Support",
      ],
      current: false,
    },
    {
      name: "Pro Professional",
      price: "$29",
      period: "month",
      description: "Perfect for growing projects & small teams.",
      features: [
        "Unlimited Managed Domains",
        "100,000 Short Links / month",
        "Up to 5 Workspace Members",
        "Real-time Geolocation Stats",
        "Catch-All Inboxes & Relays",
        "VPS Control Panel",
        "Priority Support",
      ],
      current: true,
      popular: true,
    },
    {
      name: "Business Enterprise",
      price: "$99",
      period: "month",
      description: "Dedicated resources & advanced security.",
      features: [
        "Everything in Pro Plan",
        "Unlimited Short Links",
        "Unlimited Workspace Members",
        "Dedicated Mail Relays",
        "SAML SSO / OAuth Sync",
        "99.9% SLA Guarantee",
        "24/7 Phone & Email Support",
      ],
      current: false,
    },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Billing & Plan"
        description="Monitor your subscription package, resource metrics, and payment receipts"
      />
      {/* Current plan metrics */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Active Plan</span>
            <h3 className="text-xl font-bold text-white flex items-center gap-2">
              Pro Professional
              <Badge tone="indigo">Active</Badge>
            </h3>
            <p className="text-xs text-white/50 mt-1">Renews automatically on July 15, 2026</p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4 flex items-baseline gap-1">
            <span className="text-2xl font-extrabold text-white">$29</span>
            <span className="text-xs text-white/40">/ month</span>
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Link Usage</span>
            <h3 className="text-xl font-bold text-white">42,890 / 100,000</h3>
            <p className="text-xs text-white/50 mt-1">42.8% of monthly limit consumed</p>
          </div>
          <div className="mt-6">
            <div className="h-1.5 w-full bg-white/10 rounded-full overflow-hidden">
              <div className="h-full bg-indigo-500 rounded-full" style={{ width: "42.8%" }} />
            </div>
          </div>
        </GlassCard>

        <GlassCard className="p-5 flex flex-col justify-between">
          <div>
            <span className="text-[10px] text-white/40 uppercase tracking-widest block font-bold mb-1">Active Domains</span>
            <h3 className="text-xl font-bold text-white">14 Domains</h3>
            <p className="text-xs text-white/50 mt-1">Unlimited domains allowed on Pro</p>
          </div>
          <div className="mt-6 border-t border-white/[0.06] pt-4">
            <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
              <Sparkles className="h-3.5 w-3.5" /> No limits applied
            </span>
          </div>
        </GlassCard>
      </div>

      {/* Credit Card / Payment Method */}
      <GlassCard className="p-6 space-y-4">
        <div>
          <h3 className="text-sm font-semibold text-white">Payment Method</h3>
          <p className="text-xs text-white/50">Primary card used for recurring renewals.</p>
        </div>
        <div className="flex items-center justify-between border border-white/[0.05] rounded-xl p-4 bg-black/20 max-w-md">
          <div className="flex items-center gap-3">
            <div className="h-8 w-12 bg-white/5 rounded border border-white/10 flex items-center justify-center">
              <span className="font-extrabold text-[10px] tracking-widest text-white/60">VISA</span>
            </div>
            <div>
              <span className="text-sm font-semibold text-white block">Visa ending in 4242</span>
              <span className="text-[10px] text-white/40">Expires 12/28</span>
            </div>
          </div>
          <Button variant="ghost" className="text-xs py-1 px-2.5">Edit Card</Button>
        </div>
      </GlassCard>

      {/* Plans comparison */}
      <div className="space-y-4">
        <div>
          <h3 className="text-base font-bold text-white">Upgrade or Change Plan</h3>
          <p className="text-xs text-white/50">Select a plan matching your workspace volume and infrastructure scale.</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {plans.map((p) => (
            <GlassCard key={p.name} className={`p-5 flex flex-col justify-between border-t-2 relative ${p.current ? 'border-t-indigo-500 bg-indigo-500/[0.02]' : 'border-t-white/10'}`}>
              {p.popular && (
                <span className="absolute -top-3 right-4 bg-indigo-500 text-white text-[9px] font-bold uppercase px-2 py-0.5 rounded-full shadow-glow">
                  Popular
                </span>
              )}
              <div className="space-y-4">
                <div>
                  <h4 className="font-bold text-white">{p.name}</h4>
                  <p className="text-xs text-white/40 mt-1 min-h-[32px]">{p.description}</p>
                </div>
                <div className="flex items-baseline gap-1">
                  <span className="text-3xl font-extrabold text-white">{p.price}</span>
                  <span className="text-xs text-white/40">/ {p.period}</span>
                </div>
                <ul className="space-y-2 border-t border-white/[0.06] pt-4">
                  {p.features.map((f, i) => (
                    <li key={i} className="text-xs text-white/70 flex items-center gap-2">
                      <span className="h-1 w-1 rounded-full bg-indigo-400" />
                      {f}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="mt-6 pt-2">
                {p.current ? (
                  <Button variant="subtle" className="w-full text-xs cursor-default" disabled>
                    Current Plan
                  </Button>
                ) : (
                  <Button variant={p.popular ? 'primary' : 'outline'} className="w-full text-xs">
                    Upgrade to {p.name.split(' ')[0]}
                  </Button>
                )}
              </div>
            </GlassCard>
          ))}
        </div>
      </div>

      {/* Payment History */}
      <GlassCard className="p-6 space-y-4">
        <div>
          <h3 className="text-sm font-semibold text-white">Billing History</h3>
          <p className="text-xs text-white/50">Receipts and invoices for your past payments.</p>
        </div>
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          <div className="flex justify-between items-center p-3 text-[11px] font-bold text-white/40 uppercase tracking-wider bg-white/[0.02]">
            <span className="w-24">Date</span>
            <span className="flex-1">Description</span>
            <span className="w-20 text-right">Amount</span>
            <span className="w-20 text-right">Status</span>
          </div>
          {[
            { date: "June 15, 2026", desc: "Pro Plan - Monthly Subscription Renewal", amt: "$29.00", status: "Paid" },
            { date: "May 15, 2026", desc: "Pro Plan - Monthly Subscription Renewal", amt: "$29.00", status: "Paid" },
            { date: "Apr 15, 2026", desc: "Pro Plan - Monthly Subscription Renewal", amt: "$29.00", status: "Paid" },
          ].map((item, i) => (
            <div key={i} className="flex justify-between items-center p-3 text-xs">
              <span className="w-24 text-white/60 font-mono">{item.date}</span>
              <span className="flex-1 text-white font-medium">{item.desc}</span>
              <span className="w-20 text-right text-white font-semibold font-mono">{item.amt}</span>
              <span className="w-20 text-right">
                <Badge tone="green">Paid</Badge>
              </span>
            </div>
          ))}
        </div>
      </GlassCard>
    </div>
  );
}
