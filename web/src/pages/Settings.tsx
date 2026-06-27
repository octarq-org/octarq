import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Settings, OrgMember } from "../api";
import { Empty, Field, Modal, timeAgo } from "../ui";

export default function SettingsPage() {
  const tabs = [
    { to: "/settings/general", label: "General" },
    { to: "/settings/providers", label: "Provider Accounts" },
    { to: "/settings/smtp", label: "SMTP Senders" },
    { to: "/settings/notifications", label: "Notifications" },
    { to: "/settings/members", label: "Members" },
  ];

  return (
    <div className="flex gap-8 items-start">
      <aside className="w-48 shrink-0 sticky top-6">
        <h1 className="mb-4 font-display text-xl font-bold tracking-tight text-white px-2">Settings</h1>
        <nav className="flex flex-col gap-1">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-white/[0.06] text-white"
                    : "text-white/55 hover:bg-white/[0.05] hover:text-white/75"
                }`
              }
            >
              {t.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <div className="flex-1 min-w-0 max-w-3xl">
        <Routes>
          <Route path="/" element={<Navigate to="/settings/general" replace />} />
          <Route path="/general" element={<GeneralSettings />} />
          <Route path="/providers" element={<ProviderAccounts />} />
          <Route path="/smtp" element={<SMTPSenders />} />
          <Route path="/notifications" element={<NotificationChannels />} />
          <Route path="/members" element={<OrgMembersManager />} />
        </Routes>
      </div>
    </div>
  );
}

// GeneralSettings holds runtime configuration: reserved slugs / mailboxes and a
// global Cloudflare API token used as a fallback for sync and DNS operations.
function GeneralSettings() {
  const [s, setS] = useState<Settings | null>(null);
  const [reservedSlugs, setReservedSlugs] = useState("");
  const [reservedMailboxes, setReservedMailboxes] = useState("");
  const [cfToken, setCfToken] = useState("");
  const [inboundToken, setInboundToken] = useState("");
  const [catchAll, setCatchAll] = useState(false);
  const [telegramBot, setTelegramBot] = useState("");
  const [telegramChat, setTelegramChat] = useState("");
  const [googleClientId, setGoogleClientId] = useState("");
  const [googleClientSecret, setGoogleClientSecret] = useState("");
  const [githubClientId, setGithubClientId] = useState("");
  const [githubClientSecret, setGithubClientSecret] = useState("");
  const [dataRetentionDays, setDataRetentionDays] = useState(90);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);

  async function load() {
    const v = await api.settings();
    setS(v);
    setReservedSlugs(v.reservedSlugs);
    setReservedMailboxes(v.reservedMailboxes);
    setInboundToken(v.inboundToken || "");
    setCatchAll(v.catchAll || false);
    setTelegramBot(v.telegramBotToken || "");
    setTelegramChat(v.telegramChatId || "");
    setGoogleClientId(v.googleClientId || "");
    setGithubClientId(v.githubClientId || "");
    setDataRetentionDays(v.dataRetentionDays ?? 90);
  }
  useEffect(() => {
    load();
  }, []);

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

  if (!s) return <div className="text-white/40">loading…</div>;

  return (
    <div>
      <div className="mb-4">
        <h1 className="font-display text-xl font-bold tracking-tight text-white">Settings</h1>
        <p className="text-sm text-white/40">Runtime configuration for this instance.</p>
      </div>
      <div className="card space-y-5 p-5">
        <Field
          label="Reserved slugs"
          hint={`These can't be used for short links. Always reserved: ${s.builtinReserved.join(", ")}. One per line or comma-separated.`}
        >
          <textarea
            className="input font-mono"
            rows={3}
            value={reservedSlugs}
            onChange={(e) => setReservedSlugs(e.target.value)}
            placeholder="pricing&#10;login&#10;about"
          />
        </Field>
        <Field
          label="Reserved mailbox prefixes"
          hint="Local-parts (before @) that catch-all will NOT auto-create, e.g. admin, postmaster, abuse."
        >
          <textarea
            className="input font-mono"
            rows={2}
            value={reservedMailboxes}
            onChange={(e) => setReservedMailboxes(e.target.value)}
            placeholder="admin&#10;postmaster"
          />
        </Field>
        <Field
          label="Cloudflare API token"
          hint={
            s.cloudflareTokenSet
              ? "A token is set (encrypted). Sync & DNS use it when a domain has no own token. Enter a new value to replace."
              : "Optional global token used by Sync and as a fallback for DNS operations. Zone:Read + DNS:Edit."
          }
        >
          <input
            className="input"
            value={cfToken}
            onChange={(e) => setCfToken(e.target.value)}
            placeholder={s.cloudflareTokenSet ? "•••••••• (set)" : "Cloudflare API token"}
          />
          {s.cloudflareTokenSet && (
            <button
              className="btn-ghost mt-1.5 text-red-400"
              onClick={async () => {
                await api.updateSettings({ cloudflareToken: "" });
                load();
              }}
            >
              Clear stored token
            </button>
          )}
        </Field>
        
        <div className="border-t border-white/[0.06] pt-5">
          <h2 className="mb-4 text-lg font-semibold text-white/75">Mail & Routing</h2>
          <div className="space-y-5">
            <Field
              label="Inbound Token"
              hint="Shared secret for Cloudflare Email Worker webhook (X-Led-Token)."
            >
              <input
                className="input"
                value={inboundToken}
                onChange={(e) => setInboundToken(e.target.value)}
                placeholder="secret-token"
              />
            </Field>
            
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="catchAll"
                className="accent-indigo-500"
                checked={catchAll}
                onChange={(e) => setCatchAll(e.target.checked)}
              />
              <label htmlFor="catchAll" className="text-sm cursor-pointer select-none">
                Enable Catch-All
              </label>
            </div>
            <p className="mt-1 text-xs text-white/40">
              Auto-create a mailbox when mail arrives for an unknown address on a managed domain.
            </p>
          </div>
        </div>

        <div className="border-t border-white/[0.06] pt-5">
          <h2 className="mb-4 text-lg font-semibold text-white/75">Privacy & Data Retention</h2>
          <div className="space-y-3">
            <Field label="Click Event Retention (days)" hint="Link events older than this are deleted daily. Set 0 to keep forever.">
              <input
                type="number"
                min={0}
                className="input w-32"
                value={dataRetentionDays}
                onChange={(e) => setDataRetentionDays(Number(e.target.value))}
              />
            </Field>
            <p className="text-xs text-white/40">
              IP addresses are always stored anonymized (last octet zeroed). This setting controls how long click event records are retained.
            </p>
          </div>
        </div>

        <div className="border-t border-white/[0.06] pt-5">
          <h2 className="mb-4 text-lg font-semibold text-white/75">Telegram Notifications</h2>
          <div className="space-y-5">
            <Field label="Bot Token" hint="Token from @BotFather (optional)">
              <input
                className="input"
                value={telegramBot}
                onChange={(e) => setTelegramBot(e.target.value)}
                placeholder="123456789:ABCdef..."
              />
            </Field>
            <Field label="Chat ID" hint="Your chat ID to receive notifications (optional)">
              <input
                className="input"
                value={telegramChat}
                onChange={(e) => setTelegramChat(e.target.value)}
                placeholder="123456789"
              />
            </Field>
          </div>
        </div>

        <div className="border-t border-white/[0.06] pt-5">
          <h2 className="mb-1 text-lg font-semibold text-white/75">OAuth Providers</h2>
          <p className="mb-4 text-xs text-white/40">
            Client secrets are stored encrypted. Set <code className="text-white/75">LED_BASE_URL</code> on the server for callbacks to work.
          </p>
          <div className="space-y-5">
            <div className="rounded-md border border-white/[0.06] p-4 space-y-3">
              <p className="text-sm font-medium text-white/75">Google</p>
              <Field label="Client ID" hint="">
                <input
                  className="input"
                  value={googleClientId}
                  onChange={(e) => setGoogleClientId(e.target.value)}
                  placeholder="your-client-id.apps.googleusercontent.com"
                />
              </Field>
              <Field label="Client Secret" hint={s.googleClientSecretSet ? "Secret is set (encrypted). Enter a new value to replace." : ""}>
                <input
                  className="input"
                  type="password"
                  value={googleClientSecret}
                  onChange={(e) => setGoogleClientSecret(e.target.value)}
                  placeholder={s.googleClientSecretSet ? "•••••••• (set)" : "Client secret"}
                />
                {s.googleClientSecretSet && (
                  <button
                    className="btn-ghost mt-1.5 text-red-400"
                    onClick={async () => { await api.updateSettings({ googleClientSecret: "" }); load(); }}
                  >
                    Clear secret
                  </button>
                )}
              </Field>
            </div>
            <div className="rounded-md border border-white/[0.06] p-4 space-y-3">
              <p className="text-sm font-medium text-white/75">GitHub</p>
              <Field label="Client ID" hint="">
                <input
                  className="input"
                  value={githubClientId}
                  onChange={(e) => setGithubClientId(e.target.value)}
                  placeholder="Ov23li..."
                />
              </Field>
              <Field label="Client Secret" hint={s.githubClientSecretSet ? "Secret is set (encrypted). Enter a new value to replace." : ""}>
                <input
                  className="input"
                  type="password"
                  value={githubClientSecret}
                  onChange={(e) => setGithubClientSecret(e.target.value)}
                  placeholder={s.githubClientSecretSet ? "•••••••• (set)" : "Client secret"}
                />
                {s.githubClientSecretSet && (
                  <button
                    className="btn-ghost mt-1.5 text-red-400"
                    onClick={async () => { await api.updateSettings({ githubClientSecret: "" }); load(); }}
                  >
                    Clear secret
                  </button>
                )}
              </Field>
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3 border-t border-white/[0.06] pt-5">
          <button className="btn-primary" onClick={save} disabled={busy}>
            {busy ? "Saving…" : "Save settings"}
          </button>
          {saved && <span className="text-sm text-green-400">✓ saved</span>}
        </div>
      </div>
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
      alert("Test notification sent successfully!");
    } catch (err: any) {
      alert("Test failed: " + err.message);
    }
  }

  async function toggleEnabled(c: any) {
    await api.updateNotificationChannel(c.id, { enabled: !c.enabled });
    load();
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-white">Notification Channels</h1>
          <p className="text-sm text-white/40">Channels for system alerts like inbound emails.</p>
        </div>
        <button className="btn-primary" onClick={() => setEditing({ type: "telegram", config: "{}" })}>
          + Add channel
        </button>
      </div>

      {loading ? (
        <div className="text-white/40">loading…</div>
      ) : channels.length === 0 ? (
        <Empty>
          <div className="text-2xl">🔔</div>
          <div>No notification channels yet.</div>
        </Empty>
      ) : (
        <div className="card divide-y divide-white/[0.04]">
          {channels.map((c) => (
            <div key={c.id} className="flex items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-semibold text-white/80">{c.name}</span>
                  <span className="badge">{c.type}</span>
                  {!c.enabled && <span className="badge bg-white/[0.06]">disabled</span>}
                </div>
                <div className="text-xs text-white/40 mt-0.5">Added {timeAgo(c.createdAt)}</div>
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <button
                  className="btn-ghost"
                  onClick={() => toggleEnabled(c)}
                >
                  {c.enabled ? "Disable" : "Enable"}
                </button>
                <button className="btn-ghost" onClick={() => test(c.id)}>
                  Test
                </button>
                <button className="btn-ghost" onClick={() => setEditing(c)}>
                  Edit
                </button>
                <button className="btn-ghost text-red-400" onClick={() => remove(c.id)}>
                  Delete
                </button>
              </div>
            </div>
          ))}
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
    </div>
  );
}

function EditNotificationChannel({ channel, onClose, onSaved }: { channel: any; onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState(channel?.name || "");
  const [type, setType] = useState(channel?.type || "telegram");
  const [enabled, setEnabled] = useState(channel?.id ? channel.enabled : true);

  // Parse config if editing
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
    <Modal title={channel ? "Edit Channel" : "New Channel"} onClose={onClose}>
      <div className="space-y-4">
        <Field label="Name" hint="A friendly name for this channel">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="My Telegram Bot"
          />
        </Field>
        
        <Field label="Type">
          <select className="input w-full" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="telegram">Telegram</option>
            <option value="webhook">Webhook</option>
          </select>
        </Field>

        {type === "telegram" && (
          <>
            <Field label="Bot Token" hint="Token from @BotFather">
              <input className="input w-full" value={botToken} onChange={(e) => setBotToken(e.target.value)} />
            </Field>
            <Field label="Chat ID" hint="Where to send notifications">
              <input className="input w-full" value={chatId} onChange={(e) => setChatId(e.target.value)} />
            </Field>
          </>
        )}

        {type === "webhook" && (
          <Field label="Webhook URL" hint="Receives POST requests with { text: '...' }">
            <input className="input w-full" value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} placeholder="https://..." />
          </Field>
        )}

        {error && <div className="text-red-400 text-sm">{error}</div>}

        <div className="flex justify-end gap-2 pt-2">
          <button className="btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" onClick={save} disabled={busy || !name}>
            Save
          </button>
        </div>
      </div>
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
    if (!confirm("Remove this member from the organization?")) return;
    try {
      await api.deleteOrgMember(userId);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove member");
    }
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="font-display text-xl font-bold tracking-tight text-white">Organization Members</h1>
        <p className="text-sm text-white/40">Manage who has access to this organization's resources.</p>
      </div>

      <form onSubmit={handleAdd} className="card p-4 mb-6 flex gap-3 items-end">
        <div className="flex-1">
          <label className="label">Invite Member (Email)</label>
          <input
            className="input w-full"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="colleague@example.com"
          />
        </div>
        <div className="w-32">
          <label className="label">Role</label>
          <select className="input w-full" value={role} onChange={(e) => setRole(e.target.value)}>
            <option value="member">Member</option>
            <option value="admin">Admin</option>
            <option value="owner">Owner</option>
          </select>
        </div>
        <button className="btn-primary" disabled={busy || !email}>
          Invite
        </button>
      </form>
      {err && <p className="mb-3 text-sm text-red-400">{err}</p>}

      {loading ? (
        <div className="text-white/40">loading…</div>
      ) : (
        <div className="card divide-y divide-white/[0.04]">
          {members.map((m) => (
            <div key={m.userId} className="flex justify-between items-center p-4">
              <div>
                <span className="font-semibold text-white/80">{m.email}</span>
                <span className="badge ml-2">{m.role}</span>
              </div>
              <button
                className="btn-ghost text-red-400"
                onClick={() => handleRemove(m.userId)}
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
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
    if (!confirm("Remove this provider account? Domains using it will fail to update DNS.")) return;
    try {
      await api.deleteProviderAccount(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-white">Provider Accounts</h1>
          <p className="text-sm text-white/40">
            Configure DNS providers (Cloudflare, DNSPod) used for syncing and managing domains.
          </p>
        </div>
        <button className="btn-primary" onClick={() => setCreating(true)}>+ New Account</button>
      </div>
      {loading ? (
        <div className="text-white/40">loading…</div>
      ) : accounts.length === 0 ? (
        <Empty>
          <div className="text-2xl">☁️</div>
          <div>No Provider Accounts yet.</div>
        </Empty>
      ) : (
        <div className="card divide-y divide-white/[0.04]">
          {accounts.map(a => (
            <div key={a.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-medium">{a.name}</div>
                <div className="text-xs text-white/40"><span className="badge">{a.type}</span></div>
              </div>
              <div className="flex items-center gap-4">
                <button className="btn-ghost" onClick={() => setEditing(a)}>Edit</button>
                <button className="btn-ghost text-red-400" onClick={() => remove(a.id)}>Remove</button>
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
    <Modal title={account ? "Edit Provider Account" : "New Provider Account"} onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name"><input className="input" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. My Cloudflare" autoFocus /></Field>
        {!account && (
          <Field label="Provider Type">
            <select className="input" value={type} onChange={e => setType(e.target.value)}>
              {types.map(t => <option key={t} value={t}>{t}</option>)}
            </select>
          </Field>
        )}
        <Field label="Credentials" hint={account ? "Leave empty to keep existing. Cloudflare accepts API Token string. DNSPod accepts 'ID,Token'." : "For Cloudflare, enter API Token. For DNSPod, enter 'ID,Token'."}>
          <input className="input" value={config} onChange={e => setConfig(e.target.value)} placeholder={account ? "(hidden)" : "Token..."} />
        </Field>
        {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
        <button className="btn-primary w-full" disabled={busy || !name.trim() || (!account && !config.trim())}>{busy ? "…" : "Save Account"}</button>
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
    if (!confirm("Remove this SMTP sender?")) return;
    try {
      await api.deleteSMTPSender(id);
      load();
    } catch (e: any) {
      alert(e.message || "Failed to remove");
    }
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="font-display text-xl font-bold tracking-tight text-white">SMTP Senders</h1>
          <p className="text-sm text-white/40">Configure SMTP relays for sending outgoing mail.</p>
        </div>
        <button className="btn-primary" onClick={() => setCreating(true)}>+ New Sender</button>
      </div>
      {loading ? (
        <div className="text-white/40">loading…</div>
      ) : senders.length === 0 ? (
        <Empty>
          <div className="text-2xl">📧</div>
          <div>No SMTP Senders configured yet.</div>
        </Empty>
      ) : (
        <div className="card divide-y divide-white/[0.04]">
          {senders.map(s => (
            <div key={s.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-medium">{s.name}</div>
                <div className="text-xs text-white/40">{s.fromEmail} via {s.host}:{s.port}</div>
              </div>
              <div className="flex items-center gap-4">
                <button className="btn-ghost" onClick={() => setEditing(s)}>Edit</button>
                <button className="btn-ghost text-red-400" onClick={() => remove(s.id)}>Remove</button>
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
    <Modal title={sender ? "Edit SMTP Sender" : "New SMTP Sender"} onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name"><input className="input" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Amazon SES" autoFocus /></Field>
        <Field label="Host"><input className="input" value={host} onChange={e => setHost(e.target.value)} placeholder="email-smtp.us-east-1.amazonaws.com" /></Field>
        <Field label="Port"><input type="number" className="input" value={port} onChange={e => setPort(e.target.value)} placeholder="587" /></Field>
        <Field label="Username"><input className="input" value={user} onChange={e => setUser(e.target.value)} placeholder="SMTP User" /></Field>
        <Field label="Password" hint={sender ? "Leave empty to keep existing password." : ""}>
          <input type="password" className="input" value={pass} onChange={e => setPass(e.target.value)} placeholder={sender ? "(hidden)" : "SMTP Password"} />
        </Field>
        <Field label="From Email" hint="Optional. Default address to use if none provided.">
          <input className="input" value={fromEmail} onChange={e => setFromEmail(e.target.value)} placeholder="admin@example.com" />
        </Field>
        {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
        <button className="btn-primary w-full" disabled={busy || !name.trim() || !host.trim() || !port || !user.trim() || (!sender && !pass.trim())}>{busy ? "…" : "Save Sender"}</button>
      </form>
    </Modal>
  );
}
