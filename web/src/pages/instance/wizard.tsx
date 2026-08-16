// The first-launch setup wizard. Steps are DERIVED from the readiness report
// (the fixable checks, in server order) — there is no parallel step list in
// the frontend. Completion is whatever the API says on each load; the operator
// fixes each item in the dashboard (/admin + fixPath) and this page re-reads
// the live checks. Nothing here is local state.
import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { Link } from "react-router-dom";
import { ReadinessCheck } from "../../api";
import { Alert, Badge, Button, GlassCard, PageHeader, cn } from "../../ui";
import { useTranslation } from "../../i18n";
import { fixableChecks, stepBadge, stepState, stepTone } from "./shared";

export function SetupWizard({ checks, onRefresh }: { checks: ReadinessCheck[]; onRefresh: () => void }) {
  const { t } = useTranslation();
  const steps = fixableChecks(checks);
  const done = steps.filter((s) => s.status === "ok").length;
  const blockedCount = steps.filter((s) => s.status === "blocked").length;
  const allDone = steps.length > 0 && done === steps.length;

  if (allDone) {
    return (
      <div className="space-y-6">
        <PageHeader title={t("instance.wizardTitle")} description={t("instance.wizardDesc")} />
        <GlassCard className="p-8 text-center">
          <CheckCircle2 className="mx-auto mb-3 h-8 w-8 text-success-fg" strokeWidth={1.75} />
          <h2 className="text-base font-bold text-foreground">{t("instance.wizardDoneTitle")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t("instance.wizardDoneDesc")}</p>
          <Link
            to="/"
            className="mt-5 inline-flex items-center gap-1.5 text-xs font-medium text-accent-fg hover:text-accent-fg/80"
          >
            {t("instance.openWizard")}
          </Link>
        </GlassCard>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader title={t("instance.wizardTitle")} description={t("instance.wizardDesc")} />

      {/* Blocked items break a feature RIGHT NOW (e.g. "registration is on but
          verification mail can't be sent" — new users dead-end at sign-up).
          They get a banner and a distinct danger treatment per step; degraded
          is only "an optional capability isn't configured" and stays warning. */}
      {blockedCount > 0 && (
        <Alert variant="danger" icon={<XCircle className="h-4 w-4 shrink-0" />} className="text-xs p-3 rounded-xl">
          {t("instance.blockedBanner", { count: blockedCount })}
        </Alert>
      )}

      <GlassCard className="flex flex-col gap-4 p-5 md:flex-row md:items-center md:justify-between">
        <div className="flex items-center gap-3">
          <div className="text-right">
            <span className="block text-xs text-foreground/40">{t("instance.wizardProgress", { done, total: steps.length })}</span>
          </div>
          <div className="w-32 bg-foreground/10 h-2 rounded-full overflow-hidden">
            <div
              className="h-full rounded-full bg-primary transition-all duration-500"
              style={{ width: `${steps.length ? Math.round((done / steps.length) * 100) : 0}%` }}
            />
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <Link
            to="/console"
            className="text-xs font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
          >
            {t("instance.wizardSkip")}
          </Link>
          <Button variant="outline" onClick={onRefresh}>
            <RefreshCw className="h-3.5 w-3.5" strokeWidth={1.75} />
            {t("instance.wizardRefresh")}
          </Button>
        </div>
      </GlassCard>

      <div className="space-y-3">
        {steps.map((step) => {
          const state = stepState(step.status);
          const tone = stepTone(step.status);
          const badge = stepBadge(step.status);
          return (
            <div
              key={step.id}
              data-state={state}
              className={cn(
                "flex items-start gap-3 rounded-xl border p-4",
                state === "blocked"
                  ? "border-danger-border bg-danger-bg/30"
                  : state === "degraded"
                    ? "border-warning-border/60 bg-warning-bg/20"
                    : "border-success-border/40 bg-foreground/[0.02]",
              )}
            >
              {state === "blocked" ? (
                <XCircle className="mt-0.5 h-5 w-5 shrink-0 text-danger-fg" strokeWidth={1.75} />
              ) : state === "degraded" ? (
                <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-warning-fg" strokeWidth={1.75} />
              ) : (
                <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-success-fg" strokeWidth={1.75} />
              )}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-sm font-semibold text-foreground">
                    {t(`instance.check.${step.id}`, step.title)}
                  </h3>
                  <Badge tone={badge.tone}>{t(badge.key)}</Badge>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-foreground/55">{step.detail}</p>
              </div>
              {state !== "ok" && step.fixPath && (
                <a
                  href={`/admin${step.fixPath}`}
                  className={cn(
                    "shrink-0 rounded-lg border px-3 py-1.5 text-xs font-medium transition-colors",
                    state === "blocked"
                      ? "border-danger-border bg-danger-bg text-danger-fg hover:bg-danger-fg/10"
                      : "border-warning-border bg-warning-bg text-warning-fg hover:bg-warning-fg/10",
                  )}
                >
                  {t("instance.stepFix")}
                </a>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
