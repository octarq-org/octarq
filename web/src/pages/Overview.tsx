import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../api";
import { AreaChart, BarList, timeAgo } from "../ui";
import { Header } from "./Links";

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const nav = useNavigate();

  useEffect(() => {
    api.overview().then(setO).catch(() => {});
  }, []);

  if (!o) return <div className="grid h-64 place-items-center text-zinc-500">loading…</div>;

  return (
    <div>
      <Header title="Overview" subtitle="At a glance across links, mail & domains" />

      <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card label="Total clicks" value={o.totalClicks} sub={`${o.clicks7d} in 7d`} onClick={() => nav("/links")} />
        <Card label="Links" value={o.links} sub={`${o.activeLinks} active`} onClick={() => nav("/links")} />
        <Card label="Mailboxes" value={o.mailboxes} sub={`${o.unread} unread`} onClick={() => nav("/mail")} />
        <Card label="Domains" value={o.domains} sub={`${o.linkDomains} link · ${o.mailDomains} mail`} onClick={() => nav("/domains")} />
      </div>

      <div className="mb-5 card p-4">
        <div className="mb-2 flex items-baseline justify-between">
          <h3 className="font-semibold">Clicks · last 30 days</h3>
          <span className="text-sm text-zinc-500">{o.clicks30d} total</span>
        </div>
        <AreaChart series={o.series ?? []} />
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <Panel title="Top links">
          {!o.topLinks || o.topLinks.length === 0 ? (
            <p className="text-sm text-zinc-600">No links yet</p>
          ) : (
            <div className="space-y-2">
              {o.topLinks.map((l) => (
                <button
                  key={l.id}
                  onClick={() => nav("/links")}
                  className="flex w-full items-center justify-between rounded-lg px-2 py-1.5 text-left text-sm hover:bg-zinc-800"
                >
                  <span className="truncate text-indigo-300">
                    /{l.slug}
                    {l.host && <span className="text-zinc-500"> @{l.host}</span>}
                  </span>
                  <span className="shrink-0 font-semibold">{l.clicks}</span>
                </button>
              ))}
            </div>
          )}
        </Panel>

        <Panel title="Top countries">
          <BarList rows={o.countries} empty="No geo data (set LED_GEOIP_DB)" />
        </Panel>

        <Panel title="Devices">
          <BarList rows={o.devices} />
        </Panel>
      </div>

      <div className="mt-5">
        <Panel title="Recent mail">
          {!o.recentEmails || o.recentEmails.length === 0 ? (
            <p className="text-sm text-zinc-600">No mail yet</p>
          ) : (
            <div className="divide-y divide-zinc-800">
              {o.recentEmails.map((e) => (
                <button
                  key={e.id}
                  onClick={() => nav("/mail")}
                  className="flex w-full items-center gap-3 px-2 py-2 text-left hover:bg-zinc-800"
                >
                  {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                  <span className={`w-40 shrink-0 truncate text-sm ${e.read ? "text-zinc-400" : "font-semibold"}`}>
                    {e.from || "(unknown)"}
                  </span>
                  <span className="flex-1 truncate text-sm text-zinc-400">{e.subject || "(no subject)"}</span>
                  <span className="shrink-0 text-xs text-zinc-500">{timeAgo(e.receivedAt)}</span>
                </button>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </div>
  );
}

function Card({
  label,
  value,
  sub,
  onClick,
}: {
  label: string;
  value: number;
  sub?: string;
  onClick?: () => void;
}) {
  return (
    <button onClick={onClick} className="card p-4 text-left transition hover:border-zinc-700">
      <div className="text-3xl font-bold">{value.toLocaleString()}</div>
      <div className="mt-0.5 text-sm text-zinc-400">{label}</div>
      {sub && <div className="text-xs text-zinc-600">{sub}</div>}
    </button>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="card p-4">
      <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-400">{title}</h3>
      {children}
    </div>
  );
}
