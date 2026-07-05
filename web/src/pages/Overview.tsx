import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, Overview } from "../api";
import { useTranslation } from "../i18n";
import { AreaChart, BarList, timeAgo, ScreenWrap, PageHeader, StatCard, GlassCard } from "../ui";
import { Link2, Mail, Globe, MousePointerClick, CheckCircle2, Circle, ArrowRight, Sparkles, X } from "lucide-react";

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const [includeBot, setIncludeBot] = useState(false);
  const [smtpCount, setSmtpCount] = useState<number | null>(null);
  const [memberCount, setMemberCount] = useState<number | null>(null);
  const [isPro, setIsPro] = useState(false);
  const [productCount, setProductCount] = useState<number | null>(null);
  const [priceCount, setPriceCount] = useState<number | null>(null);
  const [dismissed, setDismissed] = useState(() => localStorage.getItem("dismiss_onboarding") === "true");
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    api.overview(includeBot).then(setO).catch(() => {});
    api.smtpSenders().then(s => setSmtpCount(s.length)).catch(() => {});
    api.orgMembers().then(m => setMemberCount(m.length)).catch(() => {});
    api.license()
      .then(res => {
        setIsPro(res.licensed);
        if (res.licensed) {
          api.products().then(p => setProductCount(p.length)).catch(() => {});
          api.billingPrices().then(bp => setPriceCount(bp.length)).catch(() => {});
        }
      })
      .catch(() => setIsPro(false));
  }, [includeBot]);

  const dismiss = () => {
    localStorage.setItem("dismiss_onboarding", "true");
    setDismissed(true);
  };

  if (!o) return <div className="grid h-64 place-items-center text-white/40">{t("overview.loading")}</div>;

  const steps = [
    {
      id: "domain",
      title: t("overview.stepDomainTitle"),
      description: t("overview.stepDomainDesc"),
      completed: o.domains > 0,
      path: "/domains",
    },
    {
      id: "link",
      title: t("overview.stepLinkTitle"),
      description: t("overview.stepLinkDesc"),
      completed: o.links > 0,
      path: "/links",
    },
    {
      id: "smtp",
      title: t("overview.stepSmtpTitle"),
      description: t("overview.stepSmtpDesc"),
      completed: smtpCount !== null && smtpCount > 0,
      path: "/mail?tab=settings",
    },
    {
      id: "colleague",
      title: t("overview.stepColleagueTitle"),
      description: t("overview.stepColleagueDesc"),
      completed: memberCount !== null && memberCount > 1,
      path: "/settings/members",
    },
    ...(isPro ? [
      {
        id: "storefront",
        title: t("overview.stepStorefrontTitle"),
        description: t("overview.stepStorefrontDesc"),
        completed: productCount !== null && productCount > 0,
        path: "/storefront",
      },
      {
        id: "billing",
        title: t("overview.stepBillingTitle"),
        description: t("overview.stepBillingDesc"),
        completed: priceCount !== null && priceCount > 0,
        path: "/billing",
      }
    ] : [])
  ];

  const completedCount = steps.filter(s => s.completed).length;
  const progressPercent = Math.round((completedCount / steps.length) * 100);

  const botLabel = includeBot
    ? t("overview.botInclLabel", { count: o.botClicks30d })
    : t("overview.botHiddenLabel", { count: o.botClicks30d });

  return (
    <ScreenWrap>
      <PageHeader
        title={t("overview.title")}
        description={t("overview.description")}
      />

      {!dismissed && (
        <GlassCard className="mb-6 p-6 border-indigo-500/20 bg-indigo-950/5 relative overflow-hidden">
          <div className="absolute top-0 right-0 h-40 w-40 bg-indigo-500/5 blur-3xl rounded-full -mr-10 -mt-10 pointer-events-none" />
          
          <button 
            onClick={dismiss} 
            className="absolute top-4 right-4 p-1 rounded-lg text-white/40 hover:text-white hover:bg-white/5 transition-colors"
            title={t("overview.dismissChecklist")}
          >
            <X size={16} />
          </button>

          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
              <div className="flex items-center gap-2 text-indigo-400">
                <Sparkles size={18} className="animate-pulse" />
                <span className="text-xs font-semibold uppercase tracking-wider">{t("overview.gettingStarted")}</span>
              </div>
              <h2 className="text-xl font-bold text-white mt-1">{t("overview.maximizePerformance")}</h2>
              <p className="text-xs text-white/50 mt-1">{t("overview.gettingStartedDesc")}</p>
            </div>
            
            <div className="flex items-center gap-3 shrink-0">
              <div className="text-right">
                <span className="text-xs text-white/40">{t("overview.setupProgress")}</span>
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
          label={t("overview.totalClicks")}
          value={o.totalClicks.toLocaleString()}
          delta={t("overview.clicks7d", { count: o.clicks7d })}
          positive={true}
          icon={<MousePointerClick className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={0}
        />
        <StatCard
          label={t("overview.shortLinks")}
          value={o.links.toLocaleString()}
          delta={t("overview.activeLinks", { count: o.activeLinks })}
          positive={true}
          icon={<Link2 className="h-4 w-4" />}
          onClick={() => nav("/links")}
          index={1}
        />
        <StatCard
          label={t("overview.mailboxes")}
          value={o.mailboxes.toLocaleString()}
          delta={t("overview.unread", { count: o.unread })}
          positive={false}
          icon={<Mail className="h-4 w-4" />}
          onClick={() => nav("/mail")}
          index={2}
        />
        <StatCard
          label={t("overview.domains")}
          value={o.domains.toLocaleString()}
          delta={t("overview.domainsDelta", { link: o.linkDomains, mail: o.mailDomains })}
          positive={true}
          icon={<Globe className="h-4 w-4" />}
          onClick={() => nav("/domains")}
          index={3}
        />
      </div>

      <GlassCard className="mb-6 p-5">
        <div className="mb-4 flex items-center justify-between gap-2">
          <h3 className="font-display font-semibold text-white">{t("overview.clicksLast30")}</h3>
          <div className="flex items-center gap-3">
            <span className="text-sm text-white/40">{t("overview.clicksTotal", { count: o.clicks30d })} · {botLabel}</span>
            <BotToggle value={includeBot} onChange={setIncludeBot} />
          </div>
        </div>
        <AreaChart series={o.series ?? []} />
      </GlassCard>

      <div className="grid gap-6 lg:grid-cols-3">
        <Panel title={t("overview.topLinks")}>
          {!o.topLinks || o.topLinks.length === 0 ? (
            <p className="text-sm text-white/30">{t("overview.noLinks")}</p>
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

        <Panel title={`${t("overview.topCities")}${includeBot ? " " + t("overview.inclBots") : ""}`}>
          <BarList rows={o.cities} empty={t("overview.noGeoData")} />
        </Panel>

        <Panel title={`${t("overview.devices")}${includeBot ? " " + t("overview.inclBots") : ""}`}>
          <BarList rows={o.devices} />
        </Panel>
      </div>

      <div className="mt-6">
        <Panel title={t("overview.recentMail")}>
          {!o.recentEmails || o.recentEmails.length === 0 ? (
            <p className="text-sm text-white/30">{t("overview.noMail")}</p>
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
                    {e.from || t("overview.unknownSender")}
                  </span>
                  <span className="flex-1 truncate text-sm text-white/55">{e.subject || t("overview.noSubject")}</span>
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
  const { t } = useTranslation();
  return (
    <button
      onClick={() => onChange(!value)}
      title={value ? t("overview.hideBotTraffic") : t("overview.showBotTraffic")}
      className={`flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition ${
        value
          ? "bg-amber-500/20 text-amber-400 hover:bg-amber-500/30"
          : "bg-white/[0.06] text-white/55 hover:bg-white/[0.06]"
      }`}
    >
      <span>{value ? t("overview.botsOn") : t("overview.botsOff")}</span>
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
