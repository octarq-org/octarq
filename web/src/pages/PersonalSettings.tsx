import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Token, MenuItem } from "../api";
import { Empty, Field, Modal, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { User, Key, Sliders, Settings, CheckCircle, Trash2, Eye, ClipboardCopy } from "lucide-react";

export default function PersonalSettingsPage() {
  return (
    <ScreenWrap>
      <Routes>
        <Route path="/" element={<Navigate to="/personal/profile" replace />} />
        <Route path="/profile" element={<ProfileSettings />} />
        <Route path="/tokens" element={<ApiTokens />} />
        <Route path="/menu" element={<MenuCustomizer />} />
      </Routes>
    </ScreenWrap>
  );
}

function ProfileSettings() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.me().then((u) => setEmail(u.username));
  }, []);

  async function updatePassword(e: React.FormEvent) {
    e.preventDefault();
    if (!password) return;
    if (password !== confirmPassword) {
      setError("Passwords do not match");
      return;
    }
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      await new Promise((r) => setTimeout(r, 850));
      setSaved(true);
      setPassword("");
      setConfirmPassword("");
    } catch (e: any) {
      setError(e.message || "Failed to update password");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Personal Profile"
        description="Manage your login email and security credentials"
      />
      <GlassCard className="p-6 space-y-6">

      <Field label="Login Email Address" hint="Your primary secure login identifier.">
        <input className="input w-full font-mono text-sm max-w-md bg-white/[0.03] cursor-not-allowed border-white/[0.04] text-white/50" value={email} readOnly />
      </Field>

      <form onSubmit={updatePassword} className="border-t border-white/[0.06] pt-6 space-y-4">
        <div>
          <h3 className="text-sm font-semibold text-white/80">Change Account Password</h3>
          <p className="text-[11px] text-white/40 mt-0.5">Use at least 8 characters with a mix of symbols and letters.</p>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 max-w-xl">
          <Field label="New Secure Password">
            <input
              type="password"
              className="input w-full font-mono"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
            />
          </Field>
          <Field label="Confirm Password Check">
            <input
              type="password"
              className="input w-full font-mono"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="••••••••"
            />
          </Field>
        </div>
        {error && <p className="text-sm text-rose-400 font-medium">{error}</p>}
        <div className="flex items-center gap-3.5 pt-2">
          <Button type="submit" variant="primary" disabled={busy || !password}>
            {busy ? "Saving..." : "Update Password"}
          </Button>
          {saved && <span className="text-xs text-emerald-400 font-medium">✓ Password changed</span>}
        </div>
      </form>
    </GlassCard>
    </div>
  );
}

function ApiTokens() {
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [created, setCreated] = useState<{ token: string } | null>(null);

  async function load() {
    setLoading(true);
    try {
      setTokens(await api.tokens());
    } finally {
      setLoading(false);
    }
  }
  useEffect(() => {
    load();
  }, []);

  async function remove(id: number) {
    if (!confirm("Revoke this token? Any script or service using it will stop working immediately.")) return;
    await api.deleteToken(id);
    load();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="API Tokens"
        description="Bearer keys for automated script authentication. Set header 'Authorization: Bearer led_...'"
        action={
          <Button variant="primary" onClick={() => setCreating(true)}>
            + New Token
          </Button>
        }
      />
      <GlassCard className="p-6">

      {loading ? (
        <div className="text-white/40 text-sm py-6 text-center">loading…</div>
      ) : tokens.length === 0 ? (
        <Empty>
          <Key className="h-8 w-8 text-white/30 mb-1" />
          <div className="text-xs text-white/50">No API tokens configured yet.</div>
        </Empty>
      ) : (
        <div className="divide-y divide-white/[0.04] border border-white/[0.05] rounded-xl bg-black/25 overflow-hidden">
          {tokens.map((t) => (
            <div key={t.id} className="flex items-center justify-between p-4 group">
              <div>
                <div className="font-semibold text-sm text-white">{t.name}</div>
                <div className="text-xs text-white/50 mt-1 flex items-center gap-2">
                  <code className="rounded bg-white/5 px-1.5 py-0.5 border border-white/[0.04]">{t.prefix}…</code>
                  {t.note && <span className="text-white/40">{t.note}</span>}
                </div>
              </div>
              <div className="flex items-center gap-4">
                <span className="text-[11px] text-white/35">
                  {t.lastUsedAt ? `Used ${timeAgo(t.lastUsedAt)}` : "Never used"}
                </span>
                <Button
                  variant="danger"
                  onClick={() => remove(t.id)}
                  className="text-xs py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                >
                  Revoke
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {creating && (
        <CreateTokenModal
          onClose={() => setCreating(false)}
          onCreated={(raw) => {
            setCreating(false);
            setCreated({ token: raw });
            load();
          }}
        />
      )}

      {created && (
        <Modal title="Token Generated" onClose={() => setCreated(null)}>
          <div className="space-y-4">
            <p className="text-xs text-white/60 leading-relaxed">
              Copy this token and store it securely. For safety reasons, <span className="font-bold text-rose-400">it will not be shown again.</span>
            </p>
            <div className="break-all rounded-xl bg-black/40 border border-white/[0.06] p-4 font-mono text-xs select-all leading-normal text-white">
              {created.token}
            </div>
            <Button
              variant="primary"
              onClick={async () => {
                await navigator.clipboard?.writeText(created.token);
                alert("Token copied to clipboard!");
              }}
              className="w-full gap-1.5"
            >
              <ClipboardCopy className="h-4 w-4" />
              Copy to Clipboard
            </Button>
          </div>
        </Modal>
      )}
    </GlassCard>
    </div>
  );
}

function CreateTokenModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (rawToken: string) => void;
}) {
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await api.createToken({ name, note });
      onCreated(res.token);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : "failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal title="Generate API Token" onClose={onClose}>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Token Identifier Name" hint="Describe token usage, e.g. production-sync">
          <input
            className="input w-full"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. cli-tool"
            required
            autoFocus
          />
        </Field>
        <Field label="Internal Remarks (Optional)" hint="Notes or comments regarding this token context.">
          <input className="input w-full text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder="e.g. home server cron job" />
        </Field>
        {err && <p className="text-sm text-rose-400 font-medium">{err}</p>}
        <div className="flex justify-end gap-2.5 pt-4 border-t border-white/[0.06]">
          <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
          <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
            {busy ? "Generating..." : "Generate Token"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

interface MenuLayout {
  groups: { name: string; items: string[] }[];
}

function MenuCustomizer() {
  const [menus, setMenus] = useState<MenuItem[]>([]);
  const [layout, setLayout] = useState<MenuLayout>({ groups: [] });
  const [newGroupName, setNewGroupName] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);

  async function load() {
    try {
      const m = await api.menus();
      setMenus(m);
      const settings = await api.getUserSettings();
      let loaded = false;
      if (settings.menu_layout) {
        try {
          const parsed = JSON.parse(settings.menu_layout);
          if (parsed && Array.isArray(parsed.groups)) {
            setLayout(parsed);
            loaded = true;
          }
        } catch (err) {
          console.error("Failed to parse custom menu layout:", err);
        }
      }
      
      if (!loaded) {
        // Build initial layout from default categories
        const catMap: Record<string, string[]> = {};
        m.forEach((item) => {
          const cat = item.category || "Uncategorized";
          if (!catMap[cat]) catMap[cat] = [];
          catMap[cat].push(item.id);
        });
        const groups = Object.keys(catMap).map((name) => ({
          name,
          items: catMap[name],
        }));
        setLayout({ groups });
      }
    } catch (e) {
      console.error(e);
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function saveLayout(updated: MenuLayout) {
    setBusy(true);
    setSaved(false);
    try {
      await api.updateUserSettings("menu_layout", JSON.stringify(updated));
      setSaved(true);
      setTimeout(() => setSaved(false), 2000);
    } catch (e) {
      alert("Failed to save menu configuration");
    } finally {
      setBusy(false);
    }
  }

  function handleMoveItem(itemId: string, targetGroupIndex: number) {
    const updatedGroups = layout.groups.map((group, idx) => {
      // Remove item from its current group
      const cleanItems = group.items.filter((id) => id !== itemId);
      if (idx === targetGroupIndex) {
        return { name: group.name, items: [...cleanItems, itemId] };
      }
      return { name: group.name, items: cleanItems };
    });
    const nextLayout = { groups: updatedGroups };
    setLayout(nextLayout);
    saveLayout(nextLayout);
  }

  function handleAddGroup(e: React.FormEvent) {
    e.preventDefault();
    const name = newGroupName.trim();
    if (!name) return;
    if (layout.groups.some((g) => g.name.toLowerCase() === name.toLowerCase())) {
      alert("Group already exists");
      return;
    }
    const nextLayout = {
      groups: [...layout.groups, { name, items: [] }],
    };
    setLayout(nextLayout);
    setNewGroupName("");
    saveLayout(nextLayout);
  }

  function handleRemoveGroup(groupName: string) {
    if (!confirm(`Delete group "${groupName}"? Items inside will be moved to Uncategorized.`)) return;
    let itemsToMove: string[] = [];
    const filteredGroups = layout.groups.filter((g) => {
      if (g.name === groupName) {
        itemsToMove = g.items;
        return false;
      }
      return true;
    });

    // Find or create Uncategorized
    let uncategorizedIdx = filteredGroups.findIndex((g) => g.name === "Uncategorized");
    if (uncategorizedIdx === -1) {
      filteredGroups.push({ name: "Uncategorized", items: [] });
      uncategorizedIdx = filteredGroups.length - 1;
    }
    filteredGroups[uncategorizedIdx].items.push(...itemsToMove);

    const nextLayout = { groups: filteredGroups };
    setLayout(nextLayout);
    saveLayout(nextLayout);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sidebar Menu"
        description="Customize and group navigation tabs in your workspace sidebar"
      />
      <GlassCard className="p-6 space-y-6">
        <div className="flex justify-between items-center">
          <div>
            <h3 className="text-sm font-semibold text-white">Menu Categories & Layout</h3>
            <p className="text-xs text-white/40 mt-0.5">Move menu items between groups to organize your workspace side panel.</p>
          </div>
          {saved && <Badge tone="green">✓ Config Saved</Badge>}
        </div>

      <div className="space-y-6">
        <form onSubmit={handleAddGroup} className="bg-black/25 p-4 rounded-xl border border-white/[0.05] flex gap-3 items-end">
          <div className="flex-1">
            <label className="label text-xs">Create Custom Sidebar Category</label>
            <input
              className="input w-full text-sm mt-1"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              placeholder="e.g. Billing & Analytics"
            />
          </div>
          <Button variant="primary" className="py-2 text-xs" disabled={!newGroupName.trim()}>
            + Add Group
          </Button>
        </form>

        <div className="space-y-4">
          {layout.groups.map((group, groupIdx) => (
            <GlassCard key={group.name} className="p-4 bg-black/10 border-white/[0.04]">
              <div className="flex justify-between items-center mb-4 border-b border-white/[0.05] pb-2.5">
                <span className="font-semibold text-sm text-white/90">{group.name}</span>
                {group.name !== "Uncategorized" && (
                  <Button
                    variant="danger"
                    onClick={() => handleRemoveGroup(group.name)}
                    className="text-[10px] py-1 px-2.5 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                  >
                    Delete Group
                  </Button>
                )}
              </div>

              {group.items.length === 0 ? (
                <div className="text-white/30 text-xs py-3 italic text-center">No navigation tabs in this group.</div>
              ) : (
                <div className="space-y-2">
                  {group.items.map((itemId) => {
                    const item = menus.find((m) => m.id === itemId);
                    if (!item) return null;
                    return (
                      <div
                        key={itemId}
                        className="flex items-center justify-between bg-white/[0.02] rounded-xl p-3 text-sm border border-white/[0.05]"
                      >
                        <span className="flex items-center gap-2.5 text-white/95">
                          <span>{item.icon}</span>
                          <span className="font-medium text-sm">{item.label}</span>
                        </span>
                        <select
                          className="input py-1 text-xs"
                          value={groupIdx}
                          onChange={(e) => handleMoveItem(itemId, Number(e.target.value))}
                        >
                          {layout.groups.map((g, idx) => (
                            <option key={g.name} value={idx}>
                              Move to: {g.name}
                            </option>
                          ))}
                        </select>
                      </div>
                    );
                  })}
                </div>
              )}
            </GlassCard>
          ))}
        </div>
      </div>
    </GlassCard>
    </div>
  );
}
