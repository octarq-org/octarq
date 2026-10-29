import { Alert, Button, ExtensionSlot, ScreenWrap, PageHeader, GlassCard, Skeleton } from "@octarq/plugin-sdk";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, fetchOverview, Overview } from "../api";
import { useTranslation } from "../i18n";
import { SetupStep, SetupChecklistProvider, useSetupChecklist } from "../app-ui/SetupStep";
import { AlertCircle, Sparkles, X } from "lucide-react";
import { HealthOverview } from "./dashboard/components/HealthOverview";
import { ActivityFeed } from "./dashboard/components/ActivityFeed";

function OverviewChecklistSection({
  dismissed,
  dismiss,
  twoFAEnabled,
  tokenCount,
  nav,
  t,
}: {
  dismissed: boolean;
  dismiss: () => void;
  twoFAEnabled: boolean | null;
  tokenCount: number | null;
  nav: (path: string) => void;
  t: (key: string) => string;
}) {
  const { totalCount, progressPercent, allCompleted } = useSetupChecklist();

  if (dismissed) return null;

  // twoFAEnabled and tokenCount are null while in flight.
  const steps = [
    ...(twoFAEnabled === null
      ? []
      : [
          {
            id: "2fa",
            title: t("overview.step2FATitle"),
            description: t("overview.step2FADesc"),
            completed: twoFAEnabled === true,
            path: "/settings/security",
          },
        ]),
    ...(tokenCount === null
      ? []
      : [
          {
            id: "mcp",
            title: t("overview.stepMcpTitle"),
            description: t("overview.stepMcpDesc"),
            completed: tokenCount > 0,
            path: "/settings/tokens",
          },
        ]),
  ];

  return (
    <GlassCard className={`relative mb-8 overflow-hidden p-6 sm:p-8 ${totalCount === 0 ? "hidden" : ""}`}>
      <div aria-hidden="true" className="pointer-events-none absolute -right-24 -top-28 h-64 w-64 rounded-full bg-accent-soft blur-3xl" />
      <button 
        onClick={dismiss} 
        className="absolute top-4 right-4 p-1 rounded-lg text-muted-foreground hover:text-foreground hover:bg-well transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border"
        aria-label={t("overview.dismissChecklist")}
        title={t("overview.dismissChecklist")}
      >
        <X size={16} aria-hidden="true" />
      </button>

      <div className="relative mb-7 flex flex-col justify-between gap-5 md:flex-row md:items-end">
        <div>
          <div className={`flex items-center gap-2 ${allCompleted ? "text-success-fg" : "text-accent-fg"}`}>
            <Sparkles size={15} aria-hidden="true" />
            <span className="font-mono text-[10px] font-medium uppercase tracking-[0.1em]">{t("overview.gettingStarted")}</span>
          </div>
          <h2 className="mt-2 font-display text-[1.375rem] font-semibold tracking-[-0.02em] text-foreground">
            {allCompleted ? t("overview.allSetTitle") : t("overview.setupTitle")}
          </h2>
          <p className="mt-1.5 max-w-xl text-[12.5px] leading-relaxed text-muted-foreground">
            {allCompleted ? t("overview.allSetDesc") : t("overview.gettingStartedDesc")}
          </p>
        </div>

        <div className="w-full shrink-0 sm:w-60">
          <div className="flex items-baseline justify-between gap-3">
            <span className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">{t("overview.setupProgress")}</span>
            <span className={`font-display text-[1.375rem] font-semibold leading-none tracking-[-0.02em] ${allCompleted ? "text-success-fg" : "text-accent-fg"}`}>{progressPercent}%</span>
          </div>
          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-border">
            <div
              className={`h-full rounded-full transition-all duration-[var(--dur-slow)] ease-[var(--ease-out)] ${
                allCompleted ? "bg-success-fg" : "bg-primary"
              }`}
              style={{ width: `${progressPercent}%` }}
            />
          </div>
        </div>
      </div>

      <div className="relative grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
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
  );
}

export default function OverviewPage() {
  const [o, setO] = useState<Overview | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [twoFAEnabled, setTwoFAEnabled] = useState<boolean | null>(null);
  const [tokenCount, setTokenCount] = useState<number | null>(null);
  const [dismissed, setDismissed] = useState(() => localStorage.getItem("dismiss_onboarding") === "true");
  const nav = useNavigate();
  const { t } = useTranslation();

  // `catch(() => {})` left `o` null, which the page renders as a skeleton — a
  // failed overview request looked like an endless load. The checklist's
  // secondary requests stay neutral on failure (null hides a step rather than
  // claiming it is complete).
  function loadOverview() {
    setLoadFailed(false);
    fetchOverview()
      .then((data) => setO(data))
      .catch(() => setLoadFailed(true));
  }

  useEffect(() => {
    loadOverview();
    api.twoFAStatus().then(r => setTwoFAEnabled(r.enabled)).catch(() => {});
    api.tokens().then(toks => setTokenCount(toks.length)).catch(() => {});
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

  if (loadFailed) {
    return (
      <ScreenWrap>
        <PageHeader title={t("overview.title")} description={t("overview.description")} />
        <Alert
          variant="danger"
          icon={<AlertCircle className="h-4 w-4" />}
          actions={
            <Button variant="outline" size="sm" onClick={loadOverview}>
              {t("proTable.retry")}
            </Button>
          }
        >
          {t("overview.loadFailed")}
        </Alert>
      </ScreenWrap>
    );
  }

  if (!o) return (
    // Skeleton mirrors the real layout (title + 4 stat tiles + chart) so the page doesn't jump when data arrives.
    <div className="animate-in fade-in duration-200" aria-busy="true" aria-label={t("overview.loading")}>
      <div className="mb-6 space-y-2">
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-4 w-72" />
      </div>
      <div className="mb-8 grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="glass rounded-xl p-5">
            <Skeleton className="mb-3 h-3 w-20" />
            <Skeleton className="h-7 w-16" />
          </div>
        ))}
      </div>
      <div className="glass rounded-xl p-6">
        <Skeleton className="mb-4 h-4 w-32" />
        <Skeleton className="h-48 w-full" />
      </div>
    </div>
  );

  return (
    <ScreenWrap>
      <PageHeader
        title={t("overview.title")}
        description={t("overview.description")}
      />

      <SetupChecklistProvider>
        <OverviewChecklistSection
          dismissed={dismissed}
          dismiss={dismiss}
          twoFAEnabled={twoFAEnabled}
          tokenCount={tokenCount}
          nav={nav}
          t={t}
        />
      </SetupChecklistProvider>

      <HealthOverview />

      <ActivityFeed />

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
