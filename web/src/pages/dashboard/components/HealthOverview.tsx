import { GlassCard, Button, Alert, Skeleton } from "@octarq/plugin-sdk";
import { useMemo } from "react";
import {
  AlertCircle,
  RefreshCw,
  Wrench,
} from "lucide-react";
import { useTranslation } from "../../../i18n";
import { useHealthQuery } from "../api";
import {
  normalizeHealthStatus,
} from "../types";
import {
  HealthStatusBadge,
  StatusIcon,
  RuntimeCard,
  DatabaseCard,
  DiskCard,
  GenericProviderCard,
} from "./HealthCards";

export { HealthStatusBadge } from "./HealthCards";

export function HealthOverview() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch, isFetching, dataUpdatedAt } = useHealthQuery();

  const normalizedOverall = normalizeHealthStatus(data?.overall);

  // Collect any providers experiencing warnings or errors
  const problematicProviders = useMemo(() => {
    if (!data?.providers) return [];
    return data.providers.filter(
      (p) => normalizeHealthStatus(p.status) !== "healthy",
    );
  }, [data?.providers]);

  const hasIssues = normalizedOverall !== "healthy" || problematicProviders.length > 0;
  const isDown = normalizedOverall === "down" || problematicProviders.some((p) => normalizeHealthStatus(p.status) === "down");

  const lastUpdatedText = useMemo(() => {
    if (!dataUpdatedAt && !data?.checked_at) return "";
    const date = dataUpdatedAt ? new Date(dataUpdatedAt) : new Date(data!.checked_at);
    return date.toLocaleTimeString();
  }, [dataUpdatedAt, data?.checked_at]);

  if (isLoading && !data) {
    return (
      <div className="mb-6 space-y-4" data-testid="health-overview-loading">
        <div className="flex items-center justify-between">
          <div className="space-y-2">
            <Skeleton className="h-6 w-36" />
            <Skeleton className="h-4 w-56" />
          </div>
          <Skeleton className="h-8 w-24" />
        </div>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-48 rounded-2xl" />
          <Skeleton className="h-48 rounded-2xl" />
          <Skeleton className="h-48 rounded-2xl" />
        </div>
      </div>
    );
  }

  if (isError && !data) {
    return (
      <div data-testid="health-overview-error" className="mb-6">
        <GlassCard className="p-6">
          <Alert variant="danger" icon={<AlertCircle className="h-5 w-5" />}>
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div>
                <p className="font-semibold">{t("overview.healthLoadError")}</p>
                <p className="text-xs opacity-80 mt-0.5">{t("overview.healthTroubleshootGeneric")}</p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => refetch()}
                disabled={isFetching}
                data-testid="health-retry-btn"
                className="shrink-0"
              >
                <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${isFetching ? "animate-spin" : ""}`} />
                {t("overview.healthRetry")}
              </Button>
            </div>
          </Alert>
        </GlassCard>
      </div>
    );
  }

  const providers = data?.providers || [];
  const runtimeProvider = providers.find((p) => p.name === "runtime");
  const dbProvider = providers.find((p) => p.name === "database");
  const diskProvider = providers.find((p) => p.name === "disk");
  const customProviders = providers.filter(
    (p) => p.name !== "runtime" && p.name !== "database" && p.name !== "disk",
  );

  return (
    <div className="mb-6" data-testid="health-overview">
      {/* Header section with status badge, auto-refresh hint, and manual refresh button */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-xl bg-foreground/5">
            <StatusIcon status={normalizedOverall} />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-lg font-bold text-foreground">{t("overview.healthTitle")}</h2>
              <HealthStatusBadge status={data?.overall || "ok"} />
            </div>
            <p className="text-xs text-foreground/50">{t("overview.healthDesc")}</p>
          </div>
        </div>

        <div className="flex items-center gap-3 self-end sm:self-auto shrink-0">
          <div className="text-right hidden sm:block">
            <span className="text-[11px] text-foreground/40 block">
              {t("overview.healthAutoRefresh")}
            </span>
            {lastUpdatedText && (
              <span className="text-[11px] text-foreground/50">
                {t("overview.healthLastUpdated", { time: lastUpdatedText })}
              </span>
            )}
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
            data-testid="health-refresh-btn"
            className="h-8 text-xs font-medium"
            title={t("overview.healthManualRefresh")}
          >
            <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${isFetching ? "animate-spin" : ""}`} />
            {isFetching ? t("overview.healthRefreshing") : t("overview.healthManualRefresh")}
          </Button>
        </div>
      </div>

      {/* Troubleshooting and Alert banner when subsystem is degraded or down */}
      {hasIssues && (
        <div className="mb-4" data-testid="health-troubleshooting-alert">
          <Alert
            variant={isDown ? "danger" : "warning"}
            icon={<Wrench className="h-5 w-5" />}
          >
            <div className="space-y-2">
              <div className="font-semibold text-sm">
                {t("overview.healthAlertTitle")}
              </div>
              <div className="text-xs space-y-1.5">
                {problematicProviders.map((p) => {
                  let advice = t("overview.healthTroubleshootGeneric");
                  if (p.name === "database") {
                    advice = t("overview.healthTroubleshootDb");
                  } else if (p.name === "disk") {
                    advice = t("overview.healthTroubleshootDisk");
                  } else if (p.name === "runtime") {
                    advice = t("overview.healthTroubleshootRuntime");
                  }

                  return (
                    <div key={p.name} className="flex flex-col sm:flex-row sm:items-baseline gap-1">
                      <span className="font-semibold uppercase tracking-wider text-[10px] opacity-90">
                        [{p.name}]:
                      </span>
                      <span>{p.message ? `${p.message} — ` : ""}{advice}</span>
                    </div>
                  );
                })}
              </div>
            </div>
          </Alert>
        </div>
      )}

      {/* Provider cards grid */}
      {providers.length === 0 ? (
        <GlassCard className="p-6 text-center text-sm text-foreground/50">
          {t("overview.healthNoProviders")}
        </GlassCard>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {runtimeProvider && <RuntimeCard provider={runtimeProvider} />}
          {dbProvider && <DatabaseCard provider={dbProvider} />}
          {diskProvider && <DiskCard provider={diskProvider} />}
          {customProviders.map((p) => (
            <GenericProviderCard key={p.name} provider={p} />
          ))}
        </div>
      )}
    </div>
  );
}
