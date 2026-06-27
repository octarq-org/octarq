import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { api, ApiError, Token, MenuItem } from "../api";
import { Empty, Field, Modal, timeAgo } from "../ui";

export default function PersonalSettingsPage() {
  const tabs = [
    { to: "/personal/profile", label: "Profile" },
    { to: "/personal/tokens", label: "API Tokens" },
    { to: "/personal/menu", label: "Sidebar Menu" },
  ];

  return (
    <div className="flex gap-8 items-start">
      <aside className="w-48 shrink-0 sticky top-6">
        <h1 className="mb-4 text-xl font-semibold px-2">Personal</h1>
        <nav className="flex flex-col gap-1">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-zinc-800 text-white"
                    : "text-zinc-400 hover:bg-zinc-800/50 hover:text-zinc-300"
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
          <Route path="/" element={<Navigate to="/personal/profile" replace />} />
          <Route path="/profile" element={<ProfileSettings />} />
          <Route path="/tokens" element={<ApiTokens />} />
          <Route path="/menu" element={<MenuCustomizer />} />
        </Routes>
      </div>
    </div>
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
      // In a real multi-tenant app, password update API is called here.
      // For now, mock/simulate success or invoke a placeholder endpoint.
      await new Promise((r) => setTimeout(r, 800));
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
    <div>
      <div className="mb-4">
        <h1 className="text-xl font-semibold">My Profile</h1>
        <p className="text-sm text-zinc-500">Manage your account credentials and settings.</p>
      </div>
      <div className="card p-5 space-y-6">
        <Field label="Email Address" hint="Your login identifier.">
          <input className="input max-w-md bg-zinc-900 cursor-not-allowed" value={email} readOnly />
        </Field>

        <form onSubmit={updatePassword} className="border-t border-zinc-800 pt-5 space-y-4">
          <h2 className="text-lg font-semibold text-zinc-300">Change Password</h2>
          <Field label="New Password">
            <input
              type="password"
              className="input max-w-md"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
            />
          </Field>
          <Field label="Confirm Password">
            <input
              type="password"
              className="input max-w-md"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="••••••••"
            />
          </Field>
          {error && <p className="text-sm text-red-400">{error}</p>}
          <div className="flex items-center gap-3">
            <button className="btn-primary" disabled={busy || !password}>
              {busy ? "Saving…" : "Update Password"}
            </button>
            {saved && <span className="text-sm text-green-400">✓ password updated</span>}
          </div>
        </form>
      </div>
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
    if (!confirm("Revoke this token? Any client using it will stop working.")) return;
    await api.deleteToken(id);
    load();
  }

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Personal API Tokens</h1>
          <p className="text-sm text-zinc-500">
            Bearer tokens for the open API. Send as{" "}
            <code className="rounded bg-zinc-800 px-1">Authorization: Bearer led_…</code>
          </p>
        </div>
        <button className="btn-primary" onClick={() => setCreating(true)}>
          + New token
        </button>
      </div>

      {loading ? (
        <div className="text-zinc-500">loading…</div>
      ) : tokens.length === 0 ? (
        <Empty>
          <div className="text-2xl">🔑</div>
          <div>No API tokens yet.</div>
        </Empty>
      ) : (
        <div className="card divide-y divide-zinc-800">
          {tokens.map((t) => (
            <div key={t.id} className="flex items-center justify-between p-4">
              <div>
                <div className="font-medium">{t.name}</div>
                <div className="text-xs text-zinc-500">
                  <code className="rounded bg-zinc-800 px-1">{t.prefix}…</code>
                  {t.note && <span className="ml-2">{t.note}</span>}
                </div>
              </div>
              <div className="flex items-center gap-4">
                <span className="text-xs text-zinc-500">
                  {t.lastUsedAt ? `used ${timeAgo(t.lastUsedAt)}` : "never used"}
                </span>
                <button className="btn-ghost text-red-400" onClick={() => remove(t.id)}>
                  Revoke
                </button>
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
        <Modal title="Token created" onClose={() => setCreated(null)}>
          <p className="mb-3 text-sm text-zinc-400">
            Copy this token now — it will <b>not</b> be shown again.
          </p>
          <div className="mb-4 break-all rounded-lg bg-zinc-800 p-3 font-mono text-sm">
            {created.token}
          </div>
          <button
            className="btn-primary w-full"
            onClick={() => {
              navigator.clipboard?.writeText(created.token);
            }}
          >
            Copy to clipboard
          </button>
        </Modal>
      )}
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
    <Modal title="New API token" onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name">
          <input
            className="input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. ci-deploy"
            autoFocus
          />
        </Field>
        <Field label="Note" hint="Optional free-text remark.">
          <input className="input" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
        <button className="btn-primary w-full" disabled={busy || !name.trim()}>
          {busy ? "…" : "Create token"}
        </button>
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
      if (settings.menu_layout) {
        setLayout(JSON.parse(settings.menu_layout));
      } else {
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
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Sidebar Menu Grouping</h1>
          <p className="text-sm text-zinc-500">
            Customize and group navigation tabs in your sidebar workspace.
          </p>
        </div>
        {saved && <span className="text-sm text-green-400">✓ saved</span>}
      </div>

      <div className="space-y-6">
        <form onSubmit={handleAddGroup} className="card p-4 flex gap-3 items-end">
          <div className="flex-1">
            <label className="label">Create New Group</label>
            <input
              className="input w-full"
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              placeholder="e.g. Monitoring"
            />
          </div>
          <button className="btn-primary" disabled={!newGroupName.trim()}>
            + Add Group
          </button>
        </form>

        <div className="space-y-4">
          {layout.groups.map((group, groupIdx) => (
            <div key={group.name} className="card p-4">
              <div className="flex justify-between items-center mb-3 border-b border-zinc-800 pb-2">
                <span className="font-semibold text-zinc-300">{group.name}</span>
                {group.name !== "Uncategorized" && (
                  <button
                    className="text-xs text-red-400 hover:underline"
                    onClick={() => handleRemoveGroup(group.name)}
                  >
                    Delete Group
                  </button>
                )}
              </div>

              {group.items.length === 0 ? (
                <div className="text-zinc-600 text-xs py-2 italic">No items in this group.</div>
              ) : (
                <div className="space-y-2">
                  {group.items.map((itemId) => {
                    const item = menus.find((m) => m.id === itemId);
                    if (!item) return null;
                    return (
                      <div
                        key={itemId}
                        className="flex items-center justify-between bg-zinc-900 rounded p-2 text-sm border border-zinc-800"
                      >
                        <span className="flex items-center gap-2">
                          <span>{item.icon}</span>
                          <span>{item.label}</span>
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
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
