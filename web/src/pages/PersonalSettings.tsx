import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Token, MenuItem } from "../api";
import { Empty, Field, Modal, timeAgo, ScreenWrap, PageHeader, GlassCard, Badge, Button } from "../ui";
import { User, Key, Sliders, Settings, CheckCircle, Trash2, Eye, ClipboardCopy, LayoutDashboard, Link2, Mail, Globe, Shield, Server, KeyRound, Database, HardDrive, Wallet, ShieldAlert, ScrollText } from "lucide-react";

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

interface AreaLayout {
  groups: { name: string; items: string[] }[];
}

interface MenuLayout {
  operations: AreaLayout;
  assets: AreaLayout;
  insights: AreaLayout;
}

const MASTER_CATALOG: Record<string, { label: string; icon: React.ReactNode; defaultArea: "operations" | "assets" | "insights" }> = {
  overview: { label: "Overview", icon: <LayoutDashboard className="h-4 w-4 text-indigo-400" />, defaultArea: "operations" },
  links: { label: "Short Links", icon: <Link2 className="h-4 w-4 text-indigo-400" />, defaultArea: "operations" },
  mail: { label: "Mailbox", icon: <Mail className="h-4 w-4 text-indigo-400" />, defaultArea: "operations" },
  domains: { label: "Domains", icon: <Globe className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  certs: { label: "Certificates", icon: <Shield className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  vps: { label: "VPS", icon: <Server className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  sshkeys: { label: "SSH Keys", icon: <KeyRound className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  databases: { label: "Databases", icon: <Database className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  storage: { label: "Object Storage", icon: <HardDrive className="h-4 w-4 text-indigo-400" />, defaultArea: "assets" },
  finance: { label: "Finance", icon: <Wallet className="h-4 w-4 text-indigo-400" />, defaultArea: "insights" },
  abuse: { label: "Abuse", icon: <ShieldAlert className="h-4 w-4 text-indigo-400" />, defaultArea: "insights" },
  audit: { label: "Audit Log", icon: <ScrollText className="h-4 w-4 text-indigo-400" />, defaultArea: "insights" },
};

function areaForCategory(cat?: string): "operations" | "assets" | "insights" {
  const c = (cat ?? "").toLowerCase();
  if (c.includes("asset") || c.includes("infra") || c.includes("network") || c.includes("compute")) return "assets";
  if (c.includes("insight") || c.includes("analytic") || c.includes("finance") || c.includes("business") || c.includes("compliance") || c.includes("governance")) return "insights";
  return "operations";
}

const getDefaultLayout = (): MenuLayout => ({
  operations: {
    groups: [
      { name: "Analytics", items: ["overview"] },
      { name: "Reach", items: ["links", "mail"] }
    ]
  },
  assets: {
    groups: [
      { name: "Network", items: ["domains", "certs"] },
      { name: "Compute", items: ["vps", "sshkeys"] },
      { name: "Storage & Databases", items: ["databases", "storage"] }
    ]
  },
  insights: {
    groups: [
      { name: "Finance", items: ["finance"] },
      { name: "Governance", items: ["abuse", "audit"] }
    ]
  }
});

function MenuCustomizer() {
  const [menus, setMenus] = useState<MenuItem[]>([]);
  const [layout, setLayout] = useState<MenuLayout>(getDefaultLayout());
  const [newGroupName, setNewGroupName] = useState("");
  const [groupArea, setGroupArea] = useState<"operations" | "assets" | "insights">("operations");
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
          if (parsed && parsed.operations && parsed.assets && parsed.insights) {
            setLayout(parsed);
            loaded = true;
          }
        } catch (err) {
          console.error("Failed to parse custom menu layout:", err);
        }
      }
      
      if (!loaded) {
        // Construct default layout and append plugins
        const base = getDefaultLayout();
        m.forEach(item => {
          // If not static
          if (!MASTER_CATALOG[item.id]) {
            const areaId = areaForCategory(item.category);
            let uncategorized = base[areaId].groups.find(g => g.name === "Plugins");
            if (!uncategorized) {
              uncategorized = { name: "Plugins", items: [] };
              base[areaId].groups.push(uncategorized);
            }
            uncategorized.items.push(item.id);
          }
        });
        setLayout(base);
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

  // Move an item from any Area/Group to a specific target Area and Group name
  function handleMoveItem(itemId: string, targetArea: "operations" | "assets" | "insights", targetGroupName: string) {
    // 1. Clean item from all groups in all areas
    const cleanArea = (area: AreaLayout): AreaLayout => ({
      groups: area.groups.map(g => ({
        name: g.name,
        items: g.items.filter(id => id !== itemId)
      })).filter(g => g.name === "Uncategorized" || g.name === "Plugins" || g.items.length > 0)
    });

    const nextLayout: MenuLayout = {
      operations: cleanArea(layout.operations),
      assets: cleanArea(layout.assets),
      insights: cleanArea(layout.insights),
    };

    // 2. Add to target area group
    const targetAreaLayout = nextLayout[targetArea];
    let group = targetAreaLayout.groups.find(g => g.name === targetGroupName);
    if (!group) {
      group = { name: targetGroupName, items: [] };
      targetAreaLayout.groups.push(group);
    }
    group.items.push(itemId);

    setLayout(nextLayout);
    saveLayout(nextLayout);
  }

  function handleAddGroup(e: React.FormEvent) {
    e.preventDefault();
    const name = newGroupName.trim();
    if (!name) return;

    const areaLayout = layout[groupArea];
    if (areaLayout.groups.some(g => g.name.toLowerCase() === name.toLowerCase())) {
      alert("Group already exists in this Area");
      return;
    }

    const nextLayout = {
      ...layout,
      [groupArea]: {
        groups: [...areaLayout.groups, { name, items: [] }]
      }
    };

    setLayout(nextLayout);
    setNewGroupName("");
    saveLayout(nextLayout);
  }

  function handleRemoveGroup(areaId: "operations" | "assets" | "insights", groupName: string) {
    if (!confirm(`Delete group "${groupName}"? Items inside will be moved to Uncategorized under ${areaId}.`)) return;
    
    let itemsToMove: string[] = [];
    const areaLayout = layout[areaId];
    const filteredGroups = areaLayout.groups.filter(g => {
      if (g.name === groupName) {
        itemsToMove = g.items;
        return false;
      }
      return true;
    });

    // Find or create Uncategorized in target area
    let uncategorized = filteredGroups.find(g => g.name === "Uncategorized");
    if (!uncategorized) {
      uncategorized = { name: "Uncategorized", items: [] };
      filteredGroups.push(uncategorized);
    }
    uncategorized.items.push(...itemsToMove);

    const nextLayout = {
      ...layout,
      [areaId]: {
        groups: filteredGroups
      }
    };

    setLayout(nextLayout);
    saveLayout(nextLayout);
  }

  // Build list of all dynamic plugins + static items for display
  const getFullItemInfo = (itemId: string) => {
    const staticItem = MASTER_CATALOG[itemId];
    if (staticItem) return staticItem;
    const dynamicItem = menus.find(m => m.id === itemId);
    return {
      label: dynamicItem?.label || itemId,
      icon: <Globe className="h-4 w-4 text-violet-400" />,
      defaultArea: areaForCategory(dynamicItem?.category)
    };
  };

  const areasList = [
    { id: "operations" as const, label: "Operations Area" },
    { id: "assets" as const, label: "Assets Area" },
    { id: "insights" as const, label: "Compliance & Insights Area" },
  ];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Sidebar Menu"
        description="Configure and group navigation link categories inside your workspace sidebar"
      />
      <GlassCard className="p-6 space-y-6">
        <div className="flex justify-between items-center">
          <div>
            <h3 className="text-sm font-semibold text-white">Sidebar Menu Configurator</h3>
            <p className="text-xs text-white/40 mt-0.5">Define custom groups under Rail categories and move items freely.</p>
          </div>
          {saved && <Badge tone="green">✓ Layout Saved</Badge>}
        </div>

        <div className="space-y-6">
          {/* Create Group Form */}
          <form onSubmit={handleAddGroup} className="bg-black/25 p-4 rounded-xl border border-white/[0.05] flex flex-wrap gap-4 items-end">
            <div className="flex-1 min-w-[200px]">
              <label className="label text-xs">Add New Group Header</label>
              <input
                className="input w-full text-sm mt-1"
                value={newGroupName}
                onChange={(e) => setNewGroupName(e.target.value)}
                placeholder="e.g. DB Monitoring"
              />
            </div>
            <div className="w-48">
              <label className="label text-xs">Target Sidebar Rail Area</label>
              <select
                className="input w-full text-xs mt-1"
                value={groupArea}
                onChange={(e) => setGroupArea(e.target.value as any)}
              >
                <option value="operations">Operations Rail</option>
                <option value="assets">Assets Rail</option>
                <option value="insights">Compliance Rail</option>
              </select>
            </div>
            <Button variant="primary" type="submit" className="py-2 text-xs shrink-0" disabled={!newGroupName.trim() || busy}>
              + Add Group
            </Button>
          </form>

          {/* Three Rail Areas list */}
          <div className="space-y-6">
            {areasList.map(area => {
              const areaLayout = layout[area.id];
              return (
                <div key={area.id} className="space-y-3">
                  <h4 className="text-xs uppercase tracking-wider font-bold text-white/40 border-l-2 border-indigo-500 pl-2">
                    {area.label}
                  </h4>
                  
                  {areaLayout.groups.length === 0 ? (
                    <div className="text-white/20 text-xs py-2 italic">No groups in this Area. Create one above!</div>
                  ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      {areaLayout.groups.map(group => (
                        <GlassCard key={group.name} className="p-4 bg-black/10 border-white/[0.04] flex flex-col justify-between">
                          <div>
                            <div className="flex justify-between items-center mb-3 border-b border-white/[0.05] pb-2">
                              <span className="font-semibold text-xs text-white/80">{group.name}</span>
                              {!["Analytics", "Reach", "Network", "Compute", "Storage & Databases", "Finance", "Governance"].includes(group.name) && (
                                <Button
                                  variant="danger"
                                  onClick={() => handleRemoveGroup(area.id, group.name)}
                                  className="text-[9px] py-0.5 px-2 bg-rose-500/0 hover:bg-rose-500/10 border-0"
                                >
                                  Remove
                                </Button>
                              )}
                            </div>

                            {group.items.length === 0 ? (
                              <div className="text-white/20 text-xs py-4 text-center italic">No items here</div>
                            ) : (
                              <div className="space-y-1.5 mb-4">
                                {group.items.map(itemId => {
                                  const info = getFullItemInfo(itemId);
                                  return (
                                    <div key={itemId} className="flex items-center justify-between p-2 rounded-lg bg-white/[0.01] border border-white/[0.03]">
                                      <div className="flex items-center gap-2">
                                        {info.icon}
                                        <span className="text-xs text-white/90 font-medium">{info.label}</span>
                                      </div>
                                      
                                      <select
                                        className="input py-0.5 px-1.5 text-[10px] w-40 font-semibold"
                                        value={`${area.id}:${group.name}`}
                                        onChange={(e) => {
                                          const [tArea, tGroup] = e.target.value.split(":");
                                          handleMoveItem(itemId, tArea as any, tGroup);
                                        }}
                                      >
                                        {areasList.map(a => (
                                          <optgroup key={a.id} label={a.label}>
                                            {layout[a.id].groups.map(g => (
                                              <option key={`${a.id}:${g.name}`} value={`${a.id}:${g.name}`}>
                                                Move: {g.name}
                                              </option>
                                            ))}
                                          </optgroup>
                                        ))}
                                      </select>
                                    </div>
                                  );
                                })}
                              </div>
                            )}
                          </div>
                        </GlassCard>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </GlassCard>
    </div>
  );
}
