import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, fetchOverview, Overview } from "../api";
import { useTranslation } from "../i18n";
import { ExtensionSlot } from "../plugin-sdk";
import { ScreenWrap, PageHeader, GlassCard, Skeleton } from "../ui";
import { SetupStep } from "../components/SetupStep";
import { Sparkles, X } from "lucide-react";

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const [memberCount, setMemberCount] = useState<number | null>(null);
  const [twoFAEnabled, setTwoFAEnabled] = useState<boolean | null>(null);
  const [dismissed, setDismissed] = useState(() => localStorage.getItem("dismiss_onboarding") === "true");
  const nav = useNavigate();
  const { t } = useTranslation();

  useEffect(() => {
    fetchOverview().then(setO).catch(() => {});
    api.orgMembers().then(m => setMemberCount(m.length)).catch(() => {});
    api.twoFAStatus().then(r => setTwoFAEnabled(r.enabled)).catch(() => {});
    api.getUserSettings().then(s => {
      if (s?.onboarding_dismissed === "true") {
        setDismissed(true);
      }
    }).catch(() => {});
  }, []);

  const dismiss = () => {
    localStorage.setItem("dismiss_onboarding", "true");
    api.updateUserSettings("onboarding_dismissed", "true").catch(() => {});
    setDismissed(true);
  };

  if (!o) return (
    // Skeleton mirrors the real layout (title + 4 stat tiles + chart) so the page doesn't jump when data arrives.
    <div className="animate-in fade-in duration-200" aria-busy="true" aria-label={t("overview.loading")}>
      <div className="mb-6 space-y-2">
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-4 w-72" />
      </div>
      <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="glass rounded-2xl p-4">
            <Skeleton className="mb-3 h-3 w-20" />
            <Skeleton className="h-7 w-16" />
          </div>
        ))}
      </div>
      <div className="glass rounded-2xl p-6">
        <Skeleton className="mb-4 h-4 w-32" />
        <Skeleton className="h-48 w-full" />
      </div>
    </div>
  );

  const steps = [
    {
      id: "2fa",
      title: t("overview.step2FATitle"),
      description: t("overview.step2FADesc"),
      completed: twoFAEnabled === true,
      path: "/settings/security",
    },
    {
      id: "colleague",
      title: t("overview.stepColleagueTitle"),
      description: t("overview.stepColleagueDesc"),
      completed: memberCount !== null && memberCount > 1,
      path: "/settings/members",
    },
  ];

  const completedCount = steps.filter(s => s.completed).length;
  const progressPercent = Math.round((completedCount / steps.length) * 100);
  const allCompleted = completedCount > 0 && completedCount === steps.length;

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
            className="absolute top-4 right-4 p-1 rounded-lg text-foreground/40 hover:text-foreground hover:bg-foreground/5 transition-colors"
            title={t("overview.dismissChecklist")}
          >
            <X size={16} />
          </button>

          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
            <div>
              <div className={`flex items-center gap-2 ${allCompleted ? "text-success-fg" : "text-accent-fg"}`}>
                <Sparkles size={18} className={allCompleted ? "" : "animate-pulse"} />
                <span className="text-xs font-semibold uppercase tracking-wider">{t("overview.gettingStarted")}</span>
              </div>
              <h2 className="text-xl font-bold text-foreground mt-1">
                {allCompleted ? t("overview.allSetTitle") : t("overview.maximizePerformance")}
              </h2>
              <p className="text-xs text-foreground/50 mt-1">
                {allCompleted ? t("overview.allSetDesc") : t("overview.gettingStartedDesc")}
              </p>
            </div>
            
            <div className="flex items-center gap-3 shrink-0">
              <div className="text-right">
                <span className="text-xs text-foreground/40">{t("overview.setupProgress")}</span>
                <span className={`block text-lg font-bold ${allCompleted ? "text-success-fg" : "text-accent-fg"}`}>{progressPercent}%</span>
              </div>
              <div className="w-32 bg-foreground/10 h-2 rounded-full overflow-hidden">
                <div 
                  className={`h-full rounded-full transition-all duration-500 ${
                    allCompleted
                      ? "bg-gradient-to-r from-emerald-500 to-teal-400" /* ui-color-ok */
                      : "bg-gradient-to-r from-indigo-500 to-violet-500"
                  }`} 
                  style={{ width: `${progressPercent}%` }}
                />
              </div>
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-5">
            {steps.map((step) => (
              <SetupStep
                key={step.id}
                title={step.title}
                description={step.description}
                completed={step.completed}
                onClick={() => nav(step.path)}
              />
            ))}
            <ExtensionSlot name="home-setup-steps" />
          </div>
        </GlassCard>
      )}

      <ExtensionSlot
        name="home-overview"
        wrapper={(children) => <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-4">{children}</div>}
      />

      <ExtensionSlot name="home-chart" />

      <ExtensionSlot
        name="home-panels"
        wrapper={(children) => <div className="grid gap-6 lg:grid-cols-3">{children}</div>}
      />

      <ExtensionSlot
        name="home-rows"
        wrapper={(children) => <div className="mt-6 space-y-6">{children}</div>}
      />
    </ScreenWrap>
  );
}
