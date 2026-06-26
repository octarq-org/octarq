import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, ApiError } from "./api";
import OverviewPage from "./pages/Overview";
import LinksPage from "./pages/Links";
import DomainsPage from "./pages/Domains";
import MailPage from "./pages/Mail";
import SettingsPage from "./pages/Settings";
import SSHKeysPage from "./pages/SSHKeys";
import VPSPage from "./pages/VPS";
import FinancePage from "./pages/Finance";

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null);
  const [user, setUser] = useState("");

  useEffect(() => {
    api
      .me()
      .then((m) => {
        setUser(m.username);
        setAuthed(true);
      })
      .catch(() => setAuthed(false));
  }, []);

  if (authed === null) {
    return <div className="grid h-full place-items-center text-zinc-500">loading…</div>;
  }
  if (!authed) {
    return <Login onLogin={(u) => { setUser(u); setAuthed(true); }} />;
  }

  return (
    <div className="flex h-full">
      <Sidebar user={user} onLogout={() => setAuthed(false)} />
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
            <Route path="/settings/*" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}

function Sidebar({ user, onLogout }: { user: string; onLogout: () => void }) {
  const nav = useNavigate();
  const trafficItems = [
    { to: "/overview", label: "Overview", icon: "📊" },
    { to: "/links", label: "Links", icon: "🔗" },
    { to: "/domains", label: "Domains", icon: "🌐" },
    { to: "/mail", label: "Mail", icon: "✉️" },
  ];
  const infraItems = [
    { to: "/vps", label: "VPS", icon: "🖥️" },
    { to: "/sshkeys", label: "SSH Keys", icon: "🔑" },
    { to: "/finance", label: "Finance", icon: "💳" },
    { to: "/settings", label: "Settings", icon: "⚙️" },
  ];

  const renderNav = (items: typeof trafficItems) => (
    <nav className="flex flex-col gap-1 mb-6">
      {items.map((it) => (
        <NavLink
          key={it.to}
          to={it.to}
          className={({ isActive }) =>
            `flex items-center gap-2 rounded-lg px-3 py-2 text-sm ${
              isActive ? "bg-zinc-800 text-white" : "text-zinc-400 hover:bg-zinc-900"
            }`
          }
        >
          <span>{it.icon}</span>
          {it.label}
        </NavLink>
      ))}
    </nav>
  );

  return (
    <aside className="flex w-56 flex-col border-r border-zinc-800 bg-zinc-950 p-4 overflow-y-auto">
      <div className="mb-8 flex items-center gap-2 px-2">
        <span className="grid h-8 w-8 place-items-center rounded-lg bg-indigo-500 font-bold">l</span>
        <div>
          <div className="font-semibold leading-tight">led</div>
          <div className="text-[10px] uppercase tracking-wider text-zinc-500">link · email · domain</div>
        </div>
      </div>
      
      <div className="px-2 text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2">Traffic</div>
      {renderNav(trafficItems)}

      <div className="px-2 text-xs font-semibold text-zinc-500 uppercase tracking-wider mb-2">Infrastructure</div>
      {renderNav(infraItems)}
      <div className="mt-auto border-t border-zinc-800 pt-4">
        <div className="px-2 text-xs text-zinc-500">signed in as {user}</div>
        <button
          className="btn-ghost mt-2 w-full justify-start"
          onClick={async () => {
            await api.logout();
            onLogout();
            nav("/");
          }}
        >
          Sign out
        </button>
      </div>
    </aside>
  );
}

function Login({ onLogin }: { onLogin: (u: string) => void }) {
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
      onLogin(u);
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
