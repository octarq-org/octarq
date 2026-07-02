import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../api";
import { AreaChart, BarList, timeAgo, ScreenWrap, PageHeader, StatCard, GlassCard } from "../ui";
import { Link2, Mail, Globe, MousePointerClick, CheckCircle2, Circle, ArrowRight, Sparkles, X } from "lucide-react";

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const [includeBot, setIncludeBot] = useState(false);
  const [smtpCount, setSmtpCount] = useState<number | null>(null);
  const [memberCount, setMemberCount] = useState<number | null>(null);
  const [dismissed, setDismissed] = useState(() => localStorage.getItem("dismiss_onboarding") === "true");
  const nav = useNavigate();

  useEffect(() => {
    api.overview(includeBot).then(setO).catch(() => {});
    api.smtpSenders().then(s => setSmtpCount(s.length)).catch(() => {});
    api.orgMembers().then(m => setMemberCount(m.length)).catch(() => {});
  }, [includeBot]);

  const dismiss = () => {
    localStorage.setItem("dismiss_onboarding", "true");
    setDismissed(true);
  };

  if (!o) return <div className="grid h-64 place-items-center text-white/40">loading…</div>;

  const steps = [
    {
      id: "domain",
      title: "Domain Orchestration",
      description: "Configure a custom domain to serve branded links and secure email routes.",
      completed: o.domains > 0,
      path: "/domains",
    },
    {
      id: "link",
      title: "Branded Link Redirection",
      description: "Launch your first branded shortlink to optimize click-through conversion rates.",
      completed: o.links > 0,
      path: "/links",
    },
    {
      id: "smtp",
      title: "Outbound SMTP Relay",
      description: "Deploy SMTP relay credentials to handle secure transactional email delivery.",
      completed: smtpCount !== null && smtpCount > 0,
      path: "/settings/smtp",
    },
    {
      id: "colleague",
      title: "Multi-Tenant Collaboration",
      description: "Invite team operators to collaborate in your unified environment.",
      completed: memberCount !== null && memberCount > 1,
      path: "/settings/members",
    },
  ];

  const completedCount = steps.filter(s => s.completed).length;
  const progressPercent = Math.round((completedCount / steps.length) * 100);

  const botLabel = includeBot
    ? `incl. ${o.botClicks30d} bot`
    : `${o.botClicks30d} bot hidden`;

  return (
    <ScreenWrap>
      <PageHeader
        title="Overview"
        description="Unified analytics across links, mail & DNS"
      />

      {!dismissed && (
        <GlassCard className="mb-6 p-6 border-indigo-500/20 bg-indigo-950/5 relative overflow-hidden">
          <div className="absolute top-0 right-0 h-40 w-40 bg-indigo-500/5 blur-3xl rounded-full -mr-10 -mt-10 pointer-events-none" />
          
          <button 
            onClick={dismiss} 
            className="absolute top-4 right-4 p-1 rounded-lg text-white/40 hover:text-white hover:bg-white/5 transition-colors"
            title="Dismiss checklist"
          >
            <X size={16} />
          </button>

          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
              <div className="flex items-center gap-2 text-indigo-400">
                <Sparkles size={18} className="animate-pulse" />
                <span className="text-xs font-semibold uppercase tracking-wider">Getting Started</span>
              </div>
              <h2 className="text-xl font-bold text-white mt-1">Maximize your platform performance</h2>
              <p className="text-xs text-white/50 mt-1">Follow these steps to optimize your marketing redirection and email relay capabilities.</p>
            </div>
            
            <div className="flex items-center gap-3 shrink-0">
              <div className="text-right">
                <span className="text-xs text-white/40">Setup Progress</span>
                <span className="block text-lg font-bold text-indigo-300">{progressPercent}%</span>
              </div>
              <div className="w-32 bg-white/10 h-2 rounded-full overflow-hidden">
                <div 
                  className="bg-gradient-to-r from-indigo-500 to-violet-500 h-full rounded-full transition-all duration-500" 
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {steps.map((step) => (
              <button
                key={step.id}
                onClick={() => nav(step.path)}
                className={`group flex flex-col text-left p-4 rounded-xl border transition-all duration-200 ${
                  step.completed 
                    ? "bg-white/[0.02] border-emerald-500/20 hover:border-emerald-500/30" 
                    : "bg-white/5 border-white/[0.06] hover:border-indigo-500/30 hover:bg-white/[0.08]"
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <div className={`p-1.5 rounded-lg ${step.completed ? "text-emerald-400 bg-emerald-500/10" : "text-indigo-400 bg-indigo-500/10"}`}>
                    {step.completed ? <CheckCircle2 size={16} /> : <Circle size={16} />}
                  </div>
                  {!step.completed && (
                    <ArrowRight size={14} className="text-white/0 group-hover:text-indigo-400 translate-x-[-4px] group-hover:translate-x-0 transition-all duration-200" />
                  )}
                </div>
                <h3 className={`font-semibold text-sm mt-3 ${step.completed ? "text-white/60 line-through" : "text-white"}`}>
                  {step.title}
                </h3>
                <p className="text-[11px] text-white/40 mt-1 leading-normal flex-1">
                  {step.description}
                </p>
              </button>
            ))}
          </div>
        </GlassCard>
      )}

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
