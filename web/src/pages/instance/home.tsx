// The instance console home: a flat overview of the deployment. Top keeps the
// health summary (every readiness check, including non-actionable ones like
// database); when fixable checks need work, a blocked banner and progress bar
// summarize attention needed above the live checklist.
import { AlertTriangle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
import { ReadinessCheck } from "../../api";
import { Alert, Badge, Button, GlassCard, PageHeader, cn } from "../../ui";
import { useTranslation } from "../../i18n";
import { fixableChecks, hasFixableIssues, stepBadge, stepState } from "./shared";

export function ConsoleHome({ checks, onRefresh }: { checks: ReadinessCheck[]; onRefresh: () => void }) {
  const { t } = useTranslation();
  const steps = fixableChecks(checks);
  const done = steps.filter((s) => s.status === "ok").length;
  const blockedCount = steps.filter((s) => s.status === "blocked").length;
  const issues = hasFixableIssues(checks);
  const allOperational = !issues && checks.every((c) => c.status === "ok");

  return (
    <div className="space-y-6">
      <PageHeader
        title={
          <span className="inline-flex items-center gap-2">
            {t("instance.healthTitle")}
            {issues && <Badge tone="warning">{t("instance.needsAttention")}</Badge>}
          </span>
        }
        description={t("instance.healthDesc")}
        action={
          <Button variant="outline" onClick={onRefresh}>
            <RefreshCw className="h-3.5 w-3.5" strokeWidth={1.75} />
            {t("instance.wizardRefresh")}
          </Button>
        }
      />

      {issues && (
        <>
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
          </GlassCard>
        </>
      )}

      {allOperational && (
        <GlassCard className="flex items-center gap-3 border-success-border/60 bg-success-bg/30 p-5">
          <CheckCircle2 className="h-5 w-5 shrink-0 text-success-fg" strokeWidth={1.75} />
          <p className="text-sm font-medium text-success-fg">{t("instance.allOperational")}</p>
        </GlassCard>
      )}

      <GlassCard className="overflow-hidden !p-0">
        <div className="divide-y divide-border/40">
          {checks.map((check) => {
            const state = stepState(check.status);
            const badge = stepBadge(check.status);
            return (
              <div
                key={check.id}
                data-state={state}
                className="flex items-start gap-3 px-5 py-4"
              >
                {state === "blocked" ? (
                  <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-danger-fg" strokeWidth={1.75} />
                ) : state === "degraded" ? (
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning-fg" strokeWidth={1.75} />
                ) : (
                  <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success-fg" strokeWidth={1.75} />
                )}
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-foreground">
                      {t(`instance.check.${check.id}`, check.title)}
                    </span>
                    <Badge tone={badge.tone}>{t(badge.key)}</Badge>
                  </div>
                  <p className="mt-0.5 text-xs leading-relaxed text-foreground/55">{check.detail}</p>
                </div>
                {state !== "ok" && check.fixPath && (
                  <a
                    href={`/admin${check.fixPath}`}
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
      </GlassCard>
    </div>
  );
}
