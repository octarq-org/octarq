import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, ApiError, Org, MenuItem } from "./api";
import OverviewPage from "./pages/Overview";
import LinksPage from "./pages/Links";
import DomainsPage from "./pages/Domains";
import MailPage from "./pages/Mail";
import SettingsPage from "./pages/Settings";
import SSHKeysPage from "./pages/SSHKeys";
import VPSPage from "./pages/VPS";
import FinancePage from "./pages/Finance";
import AuditLogPage from "./pages/AuditLog";
import AbusePage from "./pages/Abuse";
import PersonalSettingsPage from "./pages/PersonalSettings";
import { Modal } from "./ui";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<number>(0);

  useEffect(() => {
    api
      .me()
      .then((m) => {
        setUser(m.username);
        setActiveOrgId(m.orgId);
        setAuthed(true);
      })
      .catch(() => setAuthed(false));
  }, []);

  if (authed === null) {
    return <div className="grid h-full place-items-center text-zinc-500">loading…</div>;
  }
  if (!authed) {
    return <Login onLogin={(u, orgId) => { setUser(u); setActiveOrgId(orgId); setAuthed(true); }} />;
  }

  return (
    <div className="flex h-full">
      <Sidebar
        user={user}
        onLogout={() => setAuthed(false)}
        activeOrgId={activeOrgId}
        setActiveOrgId={setActiveOrgId}
      />
      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-6xl p-6">
          <Routes>
            <Route path="/" element={<Navigate to="/overview" replace />} />
            <Route path="/overview" element={<OverviewPage />} />
            <Route path="/links" element={<LinksPage />} />
            <Route path="/domains" element={<DomainsPage />} />
            <Route path="/mail" element={<MailPage />} />
            <Route path="/vps" element={<VPSPage />} />
            <Route path="/sshkeys" element={<SSHKeysPage />} />
            <Route path="/finance" element={<FinancePage />} />
            <Route path="/audit" element={<AuditLogPage />} />
            <Route path="/abuse" element={<AbusePage />} />
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="/personal/*" element={<PersonalSettingsPage />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}

function Sidebar({
  user,
  onLogout,
  activeOrgId,
  setActiveOrgId,
}: {
  user: string;
  onLogout: () => void;
  activeOrgId: number;
  setActiveOrgId: (id: number) => void;
}) {
  const nav = useNavigate();
  const [orgs, setOrgs] = useState<Org[]>([]);
  const [menus, setMenus] = useState<MenuItem[]>([]);
  const [menuLayout, setMenuLayout] = useState<string>("");
  const [showSwitcher, setShowSwitcher] = useState(false);
  const [creatingOrg, setCreatingOrg] = useState(false);
  const [newOrgName, setNewOrgName] = useState("");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});

  useEffect(() => {
    // Fetch organizations
    api.orgs().then(setOrgs).catch(console.error);

    // Fetch dynamic menus
    api.menus().then(setMenus).catch(console.error);

    // Fetch user settings for menu layout
    api.getUserSettings()
      .then((settings) => {
        if (settings.menu_layout) {
          setMenuLayout(settings.menu_layout);
        }
        if (settings.collapsed_groups) {
          setCollapsedGroups(JSON.parse(settings.collapsed_groups));
        }
      })
      .catch(console.error);
  }, [activeOrgId]);

  const activeOrgName = orgs.find((o) => o.id === activeOrgId)?.name || "Personal Workspace";

  function handleSwitchOrg(orgId: number) {
    api.switchOrg(orgId).then(() => {
      setActiveOrgId(orgId);
      setShowSwitcher(false);
      window.location.reload();
    }).catch((e) => alert(e.message || "Failed to switch organization"));
  }

  function handleCreateOrg(e: React.FormEvent) {
    e.preventDefault();
    if (!newOrgName.trim()) return;
    api.createOrg({ name: newOrgName })
      .then((org) => {
        api.switchOrg(org.id).then(() => {
          window.location.reload();
        });
      })
      .catch((e) => alert(e.message || "Failed to create organization"));
  }

  function toggleGroup(groupName: string) {
    const nextCollapsed = {
      ...collapsedGroups,
      [groupName]: !collapsedGroups[groupName],
    };
    setCollapsedGroups(nextCollapsed);
    api.updateUserSettings("collapsed_groups", JSON.stringify(nextCollapsed)).catch(console.error);
  }

  // Group menus based on custom layout or category field
  let groupedMenus: { name: string; items: MenuItem[] }[] = [];
  if (menuLayout) {
    try {
      const layout = JSON.parse(menuLayout);
      const assignedIds = new Set<string>();
      groupedMenus = layout.groups.map((g: any) => {
        const items = g.items
          .map((id: string) => menus.find((m) => m.id === id))
          .filter(Boolean) as MenuItem[];
        items.forEach((item) => assignedIds.add(item.id));
        return { name: g.name, items };
      });
      // Handle unassigned menus (e.g. newly registered plugins)
      const unassigned = menus.filter((m) => !assignedIds.has(m.id));
      if (unassigned.length > 0) {
        const uncategorizedIdx = groupedMenus.findIndex((g) => g.name === "Uncategorized");
        if (uncategorizedIdx > -1) {
          groupedMenus[uncategorizedIdx].items.push(...unassigned);
        } else {
          groupedMenus.push({ name: "Uncategorized", items: unassigned });
        }
      }
    } catch (e) {
      console.error(e);
    }
  }

  // Fallback to grouping by category field from API
  if (groupedMenus.length === 0 && menus.length > 0) {
    const catMap: Record<string, MenuItem[]> = {};
    menus.forEach((item) => {
      const cat = item.category || "Uncategorized";
      if (!catMap[cat]) catMap[cat] = [];
      catMap[cat].push(item);
    });
    groupedMenus = Object.keys(catMap).map((name) => ({
      name,
      items: catMap[name],
    }));
  }

  return (
    <aside className="flex w-56 flex-col border-r border-zinc-800 bg-zinc-950 p-4 overflow-y-auto">
      {/* Organization Switcher Header */}
      <div className="relative mb-6">
        <button
          onClick={() => setShowSwitcher(!showSwitcher)}
          className="flex w-full items-center justify-between rounded-lg border border-zinc-800 bg-zinc-900/50 p-2.5 hover:bg-zinc-800 transition-colors text-left"
        >
          <div className="min-w-0 flex-1">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-zinc-500 leading-none mb-1">
              Organization
            </div>
            <div className="font-semibold text-sm text-zinc-200 truncate leading-snug">
              {activeOrgName}
            </div>
          </div>
          <span className="text-zinc-500 text-xs ml-1">▼</span>
        </button>

        {showSwitcher && (
          <div className="absolute top-full left-0 z-50 mt-1 w-full rounded-lg border border-zinc-800 bg-zinc-900 p-1.5 shadow-xl">
            <div className="max-h-48 overflow-y-auto space-y-0.5">
              {orgs.map((o) => (
                <button
                  key={o.id}
                  onClick={() => handleSwitchOrg(o.id)}
                  className={`flex w-full items-center justify-between rounded px-2 py-1.5 text-xs font-medium transition-colors ${
                    o.id === activeOrgId
                      ? "bg-zinc-800 text-white"
                      : "text-zinc-400 hover:bg-zinc-800/50 hover:text-zinc-300"
                  }`}
                >
                  <span className="truncate">{o.name}</span>
                  {o.id === activeOrgId && <span className="text-indigo-400">✓</span>}
                </button>
              ))}
            </div>
            <div className="mt-1.5 border-t border-zinc-800 pt-1.5">
              <button
                onClick={() => {
                  setCreatingOrg(true);
                  setShowSwitcher(false);
                }}
                className="flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-xs font-medium text-indigo-400 hover:bg-zinc-800/50"
              >
                <span>+</span> Create Organization
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Grouped Dynamic Menus */}
      <div className="flex-1 space-y-4">
        {groupedMenus.map((group) => {
          const isCollapsed = !!collapsedGroups[group.name];
          return (
            <div key={group.name} className="space-y-1">
              <button
                onClick={() => toggleGroup(group.name)}
                className="flex w-full items-center justify-between px-2 text-[10px] font-semibold text-zinc-500 uppercase tracking-wider hover:text-zinc-400"
              >
                <span>{group.name}</span>
                <span className="text-[8px]">{isCollapsed ? "▶" : "▼"}</span>
              </button>

              {!isCollapsed && (
                <nav className="flex flex-col gap-0.5 mt-1">
                  {group.items.map((it) => (
                    <NavLink
                      key={it.id}
                      to={it.path}
                      className={({ isActive }) =>
                        `flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-all ${
                          isActive
                            ? "bg-zinc-800 text-white shadow-inner"
                            : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-300"
                        }`
                      }
                    >
                      <span className="text-sm shrink-0">{it.icon}</span>
                      <span className="truncate">{it.label}</span>
                    </NavLink>
                  ))}
                </nav>
              )}
            </div>
          );
        })}
      </div>

      {/* Personal Settings Footer */}
      <div className="mt-auto border-t border-zinc-800 pt-4 space-y-2">
        <NavLink
          to="/personal"
          className={({ isActive }) =>
            `flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-xs font-medium transition-colors ${
              isActive
                ? "bg-zinc-800 text-white"
                : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-300"
            }`
          }
        >
          <span className="text-sm shrink-0">👤</span>
          <div className="min-w-0 flex-1">
            <div className="font-semibold truncate leading-tight">Personal Workspace</div>
            <div className="text-[10px] text-zinc-500 truncate leading-none mt-0.5">{user}</div>
          </div>
        </NavLink>

        <button
          className="btn-ghost w-full justify-start text-xs font-medium py-1.5"
          onClick={async () => {
            await api.logout();
            onLogout();
            nav("/");
          }}
        >
          Sign out
        </button>
      </div>

      {/* Create Org Modal */}
      {creatingOrg && (
        <Modal title="Create Organization" onClose={() => setCreatingOrg(false)}>
          <form onSubmit={handleCreateOrg} className="space-y-4">
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-zinc-400">Organization Name</label>
              <input
                className="input w-full"
                value={newOrgName}
                onChange={(e) => setNewOrgName(e.target.value)}
                placeholder="e.g. Acme Corporation"
                autoFocus
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                className="btn-ghost"
                onClick={() => setCreatingOrg(false)}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="btn-primary"
                disabled={!newOrgName.trim()}
              >
                Create & Switch
              </button>
            </div>
          </form>
        </Modal>
      )}
    </aside>
  );
}

function Login({ onLogin }: { onLogin: (u: string, orgId: number) => void }) {
  const [u, setU] = useState("admin");
  const [p, setP] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await api.login(u, p);
      const m = await api.me();
      onLogin(u, m.orgId);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : "login failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid h-full place-items-center">
      <form onSubmit={submit} className="card w-80 p-6">
        <div className="mb-6 text-center">
          <div className="mx-auto mb-2 grid h-10 w-10 place-items-center rounded-xl bg-indigo-500 text-lg font-bold">
            l
          </div>
          <h1 className="text-lg font-semibold">Sign in to led</h1>
        </div>
        <label className="label">Username</label>
        <input className="input mb-3" value={u} onChange={(e) => setU(e.target.value)} />
        <label className="label">Password</label>
        <input
          type="password"
          className="input mb-4"
          value={p}
          onChange={(e) => setP(e.target.value)}
          autoFocus
        />
        {err && <p className="mb-3 text-sm text-red-400">{err}</p>}
        <button className="btn-primary w-full" disabled={busy}>
          {busy ? "…" : "Sign in"}
        </button>
        <div className="mt-4 flex flex-col gap-2">
          <div className="flex items-center gap-2 text-xs text-zinc-500">
            <span className="h-px flex-1 bg-zinc-700" />
            or
            <span className="h-px flex-1 bg-zinc-700" />
          </div>
          <a
            href="/auth/begin/google"
            className="flex items-center justify-center gap-2 rounded-md border border-zinc-700 px-3 py-2 text-sm text-zinc-300 hover:bg-zinc-800 transition-colors"
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z" fill="#4285F4"/>
              <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" fill="#34A853"/>
              <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l3.66-2.84z" fill="#FBBC05"/>
              <path d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" fill="#EA4335"/>
            </svg>
            Continue with Google
          </a>
          <a
            href="/auth/begin/github"
            className="flex items-center justify-center gap-2 rounded-md border border-zinc-700 px-3 py-2 text-sm text-zinc-300 hover:bg-zinc-800 transition-colors"
          >
            <svg className="h-4 w-4" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
            </svg>
            Continue with GitHub
          </a>
        </div>
      </form>
    </div>
  );
}
