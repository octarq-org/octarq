import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes, useNavigate } from "react-router-dom";
import { api, ApiError } from "./api";
import OverviewPage from "./pages/Overview";
import LinksPage from "./pages/Links";
import DomainsPage from "./pages/Domains";
import MailPage from "./pages/Mail";
import SettingsPage from "./pages/Settings";

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
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Routes>
        </div>
      </main>
    </div>
  );
}

function Sidebar({ user, onLogout }: { user: string; onLogout: () => void }) {
  const nav = useNavigate();
  const items = [
    { to: "/overview", label: "Overview", icon: "📊" },
    { to: "/links", label: "Links", icon: "🔗" },
    { to: "/domains", label: "Domains", icon: "🌐" },
    { to: "/mail", label: "Mail", icon: "✉️" },
    { to: "/settings", label: "Settings", icon: "⚙️" },
  ];
  return (
    <aside className="flex w-56 flex-col border-r border-zinc-800 bg-zinc-950 p-4">
      <div className="mb-8 flex items-center gap-2 px-2">
        <span className="grid h-8 w-8 place-items-center rounded-lg bg-indigo-500 font-bold">l</span>
        <div>
          <div className="font-semibold leading-tight">led</div>
          <div className="text-[10px] uppercase tracking-wider text-zinc-500">link · email · domain</div>
        </div>
      </div>
      <nav className="flex flex-col gap-1">
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
      </form>
    </div>
  );
}
