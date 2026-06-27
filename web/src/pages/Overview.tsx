import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../api";
import { AreaChart, BarList, timeAgo, ScreenWrap, PageHeader, StatCard, GlassCard } from "../ui";
import { Link2, Mail, Globe, MousePointerClick } from "lucide-react";

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const [includeBot, setIncludeBot] = useState(false);
  const nav = useNavigate();

  useEffect(() => {
    api.overview(includeBot).then(setO).catch(() => {});
  }, [includeBot]);

  if (!o) return <div className="grid h-64 place-items-center text-white/40">loading…</div>;

  const botLabel = includeBot
    ? `incl. ${o.botClicks30d} bot`
    : `${o.botClicks30d} bot hidden`;

  return (
    <ScreenWrap>
      <PageHeader
        title="Overview"
        description="At a glance across links, mail & domains"
      />

      <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          label="Total Clicks"
          value={o.totalClicks.toLocaleString()}
          delta={`${o.clicks7d} in 7d`}
          positive={true}
          icon={<MousePointerClick className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={0}
        />
        <StatCard
          label="Short Links"
          value={o.links.toLocaleString()}
          delta={`${o.activeLinks} active`}
          positive={true}
          icon={<Link2 className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={1}
        />
        <StatCard
          label="Mailboxes"
          value={o.mailboxes.toLocaleString()}
          delta={`${o.unread} unread`}
          positive={false}
          icon={<Mail className="h-4 w-4" />}
          onClick={() => nav("/mail")}
          index={2}
        />
        <StatCard
          label="Domains"
          value={o.domains.toLocaleString()}
          delta={`${o.linkDomains} link · ${o.mailDomains} mail`}
          positive={true}
          icon={<Globe className="h-4 w-4" />}
          onClick={() => nav("/domains")}
          index={3}
        />
      </div>

      <GlassCard className="mb-6 p-5">
        <div className="mb-4 flex items-center justify-between gap-2">
          <h3 className="font-display font-semibold text-white">Clicks · last 30 days</h3>
          <div className="flex items-center gap-3">
            <span className="text-sm text-white/40">{o.clicks30d} total · {botLabel}</span>
            <BotToggle value={includeBot} onChange={setIncludeBot} />
          </div>
        </div>
        <AreaChart series={o.series ?? []} />
      </GlassCard>

      <div className="grid gap-6 lg:grid-cols-3">
        <Panel title="Top links">
          {!o.topLinks || o.topLinks.length === 0 ? (
            <p className="text-sm text-white/30">No links yet</p>
          ) : (
            <div className="space-y-1">
              {o.topLinks.map((l) => (
                <button
                  key={l.id}
                  onClick={() => nav("/links")}
                  className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-sm hover:bg-white/[0.06] transition-colors"
                >
                  <span className="truncate text-indigo-300">
                    /{l.slug}
                    {l.host && <span className="text-white/40"> @{l.host}</span>}
                  </span>
                  <span className="shrink-0 font-semibold">{l.clicks}</span>
                </button>
              ))}
            </div>
          )}
        </Panel>

        <Panel title={`Top countries${includeBot ? " (incl. bots)" : ""}`}>
          <BarList rows={o.countries} empty="No geo data (set LED_GEOIP_DB)" />
        </Panel>

        <Panel title={`Devices${includeBot ? " (incl. bots)" : ""}`}>
          <BarList rows={o.devices} />
        </Panel>
      </div>

      <div className="mt-6">
        <Panel title="Recent mail">
          {!o.recentEmails || o.recentEmails.length === 0 ? (
            <p className="text-sm text-white/30">No mail yet</p>
          ) : (
            <div className="divide-y divide-white/[0.04]">
              {o.recentEmails.map((e) => (
                <button
                  key={e.id}
                  onClick={() => nav("/mail")}
                  className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-white/[0.06] transition-colors"
                >
                  {!e.read && <span className="h-2 w-2 shrink-0 rounded-full bg-indigo-400" />}
                  <span className={`w-40 shrink-0 truncate text-sm ${e.read ? "text-white/55" : "font-semibold"}`}>
                    {e.from || "(unknown)"}
                  </span>
                  <span className="flex-1 truncate text-sm text-white/55">{e.subject || "(no subject)"}</span>
                  <span className="shrink-0 text-xs text-white/40">{timeAgo(e.receivedAt)}</span>
                </button>
              ))}
            </div>
          )}
        </Panel>
      </div>
    </ScreenWrap>
  );
}

function BotToggle({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!value)}
      title={value ? "Hide bot traffic" : "Show bot traffic"}
      className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition ${
        value
          ? "bg-amber-500/20 text-amber-400 hover:bg-amber-500/30"
          : "bg-white/[0.06] text-white/55 hover:bg-white/[0.06]"
      }`}
    >
      <span>{value ? "🤖 bots on" : "🤖 bots off"}</span>
    </button>
  );
}

function Panel({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <GlassCard className="p-5">
      <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-white/35">{title}</h3>
      {children}
    </GlassCard>
  );
}
