import { useMemo } from "react";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Database,
  HardDrive,
  RefreshCw,
  Wrench,
} from "lucide-react";
import { useTranslation } from "../../../i18n";
import { GlassCard, Badge, Button, Alert, Skeleton } from "../../../ui";
import { useHealthQuery } from "../api";
import {
  formatBytes,
  formatDuration,
  normalizeHealthStatus,
  NormalizedHealthStatus,
  ProviderHealthReport,
} from "../types";

export function HealthStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const normalized = normalizeHealthStatus(status);

  switch (normalized) {
    case "healthy":
      return (
        <Badge tone="success" shape="dot">
          {t("overview.healthStatusHealthy")}
        </Badge>
      );
    case "degraded":
      return (
        <Badge tone="warning" shape="dot">
          {t("overview.healthStatusDegraded")}
        </Badge>
      );
    case "down":
      return (
        <Badge tone="danger" shape="dot">
          {t("overview.healthStatusDown")}
        </Badge>
      );
    default:
      return (
        <Badge tone="default" shape="dot">
          {t("overview.healthStatusHealthy")}
        </Badge>
      );
  }
}

function StatusIcon({ status }: { status: NormalizedHealthStatus }) {
  switch (status) {
    case "healthy":
      return <CheckCircle2 className="h-5 w-5 text-success-fg shrink-0" />;
    case "degraded":
      return <AlertTriangle className="h-5 w-5 text-warning-fg shrink-0" />;
    case "down":
      return <AlertCircle className="h-5 w-5 text-danger-fg shrink-0" />;
    default:
      return <Activity className="h-5 w-5 text-muted-foreground shrink-0" />;
  }
}

function RuntimeCard({ provider }: { provider: ProviderHealthReport }) {
  const { t } = useTranslation();
  const metrics = provider.metrics || {};

  const goroutines =
    typeof metrics.goroutines === "number" ? metrics.goroutines.toLocaleString() : "-";
  const heapAlloc = formatBytes(metrics.heap_alloc_bytes ?? metrics.alloc_bytes);
  const heapSys = formatBytes(metrics.heap_sys_bytes ?? metrics.sys_bytes);
  const uptime = formatDuration(metrics.uptime_seconds);
  const goVersion = typeof metrics.go_version === "string" ? metrics.go_version : null;
  const numCpu = typeof metrics.num_cpu === "number" ? metrics.num_cpu : null;

  return (
    <div data-testid="health-card-runtime" className="h-full">
      <GlassCard className="p-5 flex flex-col justify-between h-full">
      <div>
        <div className="flex items-center justify-between gap-2 mb-4">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-primary/10 text-primary">
              <Cpu className="h-5 w-5" />
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-sm">
                {t("overview.healthProviderRuntime")}
              </h3>
              {goVersion && (
                <p className="text-[11px] text-foreground/50">
                  {goVersion}
                  {numCpu ? ` · ${numCpu} ${t("overview.healthCpuCores")}` : ""}
                </p>
              )}
            </div>
          </div>
          <HealthStatusBadge status={provider.status} />
        </div>

        <div className="grid grid-cols-2 gap-3 mb-3">
          <div className="rounded-xl bg-foreground/5 p-3">
            <span className="text-xs text-foreground/50 block mb-1">
              {t("overview.healthGoroutines")}
            </span>
            <span className="text-lg font-bold text-foreground font-mono">{goroutines}</span>
          </div>
          <div className="rounded-xl bg-foreground/5 p-3">
            <span className="text-xs text-foreground/50 block mb-1">
              {t("overview.healthUptime")}
            </span>
            <span className="text-lg font-bold text-foreground font-mono">{uptime}</span>
          </div>
        </div>

        <div className="rounded-xl bg-foreground/5 p-3">
          <div className="flex items-center justify-between text-xs mb-1.5">
            <span className="text-foreground/50">{t("overview.healthMemory")}</span>
            <span className="font-mono text-foreground/70">{heapAlloc}</span>
          </div>
          <div className="flex items-center justify-between text-[11px] text-foreground/40 font-mono">
            <span>{t("overview.healthHeap", { size: heapAlloc })}</span>
            <span>{t("overview.healthSys", { size: heapSys })}</span>
          </div>
        </div>
      </div>
    </GlassCard>
    </div>
  );
}

function DatabaseCard({ provider }: { provider: ProviderHealthReport }) {
  const { t } = useTranslation();
  const metrics = provider.metrics || {};

  const openConns = typeof metrics.open_connections === "number" ? metrics.open_connections : 0;
  const maxConns =
    typeof metrics.max_open_connections === "number" && metrics.max_open_connections > 0
      ? metrics.max_open_connections
      : null;
  const inUse = typeof metrics.in_use === "number" ? metrics.in_use : 0;
  const idle = typeof metrics.idle === "number" ? metrics.idle : 0;

  const pingLatency =
    typeof metrics.ping_latency_ms === "number" ? `${metrics.ping_latency_ms} ms` : "< 1 ms";
  const slowQueries = typeof metrics.slow_queries === "number" ? metrics.slow_queries : 0;

  return (
    <div data-testid="health-card-database" className="h-full">
      <GlassCard className="p-5 flex flex-col justify-between h-full">
      <div>
        <div className="flex items-center justify-between gap-2 mb-4">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-accent-fg/10 text-accent-fg">
              <Database className="h-5 w-5" />
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-sm">
                {t("overview.healthProviderDatabase")}
              </h3>
              <p className="text-[11px] text-foreground/50">
                {inUse} {t("overview.healthInUse")} · {idle} {t("overview.healthIdle")}
              </p>
            </div>
          </div>
          <HealthStatusBadge status={provider.status} />
        </div>

        <div className="grid grid-cols-2 gap-3 mb-3">
          <div className="rounded-xl bg-foreground/5 p-3">
            <span className="text-xs text-foreground/50 block mb-1">
              {t("overview.healthConnections")}
            </span>
            <div className="flex items-baseline gap-1 font-mono">
              <span className="text-lg font-bold text-foreground">{openConns}</span>
              {maxConns !== null && (
                <span className="text-xs text-foreground/40">/ {maxConns}</span>
              )}
            </div>
          </div>
          <div className="rounded-xl bg-foreground/5 p-3">
            <span className="text-xs text-foreground/50 block mb-1">
              {t("overview.healthPingLatency")}
            </span>
            <span className="text-lg font-bold text-foreground font-mono">{pingLatency}</span>
          </div>
        </div>

        <div className="rounded-xl bg-foreground/5 p-3 flex items-center justify-between">
          <span className="text-xs text-foreground/50">{t("overview.healthSlowQueries")}</span>
          <span
            className={`font-mono text-xs font-semibold px-2 py-0.5 rounded-md ${
              slowQueries > 0
                ? "bg-warning-bg text-warning-fg border border-warning-border"
                : "text-foreground/70"
            }`}
          >
            {slowQueries}
          </span>
        </div>
      </div>
    </GlassCard>
    </div>
  );
}

function DiskCard({ provider }: { provider: ProviderHealthReport }) {
  const { t } = useTranslation();
  const metrics = provider.metrics || {};

  const usagePercent =
    typeof metrics.usage_percent === "number"
      ? Number(metrics.usage_percent.toFixed(1))
      : 0;
  const used = formatBytes(metrics.used_bytes);
  const total = formatBytes(metrics.total_bytes);
  const free = formatBytes(metrics.free_bytes ?? metrics.available_bytes);
  const diskPath = typeof metrics.path === "string" ? metrics.path : null;

  // Determine progress bar color based on standard thresholds (warn 85%, error 95%)
  const barColor =
    usagePercent >= 95
      ? "bg-danger-fg"
      : usagePercent >= 85
      ? "bg-warning-fg"
      : "bg-success-fg";

  return (
    <div data-testid="health-card-disk" className="h-full">
      <GlassCard className="p-5 flex flex-col justify-between h-full">
      <div>
        <div className="flex items-center justify-between gap-2 mb-4">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-info-fg/10 text-info-fg">
              <HardDrive className="h-5 w-5" />
            </div>
            <div>
              <h3 className="font-semibold text-foreground text-sm">
                {t("overview.healthProviderDisk")}
              </h3>
              {diskPath && (
                <p className="text-[11px] text-foreground/50 truncate max-w-[160px]" title={diskPath}>
                  {diskPath}
                </p>
              )}
            </div>
          </div>
          <HealthStatusBadge status={provider.status} />
        </div>

        <div className="rounded-xl bg-foreground/5 p-3 mb-3">
          <div className="flex items-center justify-between text-xs mb-2">
            <span className="text-foreground/50">{t("overview.healthDiskUsage")}</span>
            <span className="font-mono font-bold text-foreground text-sm">{usagePercent}%</span>
          </div>
          <div
            className="w-full bg-foreground/10 h-2 rounded-full overflow-hidden"
            role="progressbar"
            aria-valuenow={usagePercent}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            <div
              className={`h-full rounded-full transition-all duration-500 ${barColor}`}
              style={{ width: `${Math.min(100, Math.max(0, usagePercent))}%` }}
            />
          </div>
        </div>

        <div className="grid grid-cols-3 gap-2 text-xs font-mono">
          <div className="rounded-xl bg-foreground/5 p-2.5">
            <span className="text-[11px] text-foreground/40 block mb-0.5">
              {t("overview.healthDiskUsed")}
            </span>
            <span className="text-foreground/80 truncate block">{used}</span>
          </div>
          <div className="rounded-xl bg-foreground/5 p-2.5">
            <span className="text-[11px] text-foreground/40 block mb-0.5">
              {t("overview.healthDiskFree")}
            </span>
            <span className="text-foreground/80 truncate block">{free}</span>
          </div>
          <div className="rounded-xl bg-foreground/5 p-2.5">
            <span className="text-[11px] text-foreground/40 block mb-0.5">
              {t("overview.healthDiskTotal")}
            </span>
            <span className="text-foreground/80 truncate block">{total}</span>
          </div>
        </div>
      </div>
    </GlassCard>
    </div>
  );
}

function GenericProviderCard({ provider }: { provider: ProviderHealthReport }) {
  return (
    <div data-testid={`health-card-${provider.name}`} className="h-full">
      <GlassCard className="p-5 flex flex-col justify-between h-full">
        <div>
          <div className="flex items-center justify-between gap-2 mb-3">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-primary" />
              <h3 className="font-semibold text-foreground text-sm capitalize">{provider.name}</h3>
            </div>
            <HealthStatusBadge status={provider.status} />
          </div>
          {provider.message && (
            <p className="text-xs text-foreground/60 mb-3">{provider.message}</p>
          )}
          <div className="space-y-1.5 text-xs font-mono">
            {Object.entries(provider.metrics || {}).map(([k, v]) => (
              <div key={k} className="flex justify-between py-1 border-b border-border/40">
                <span className="text-foreground/50">{k}:</span>
                <span className="text-foreground/80">{String(v)}</span>
              </div>
            ))}
          </div>
        </div>
      </GlassCard>
    </div>
  );
}

export function HealthOverview() {
  const { t } = useTranslation();
  const { data, isLoading, isError, refetch, isFetching, dataUpdatedAt } = useHealthQuery();

  const normalizedOverall = normalizeHealthStatus(data?.overall);

  // Collect any providers experiencing warnings or errors
  const problematicProviders = useMemo(() => {
    if (!data?.providers) return [];
    return data.providers.filter(
      (p) => normalizeHealthStatus(p.status) !== "healthy"
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
    (p) => p.name !== "runtime" && p.name !== "database" && p.name !== "disk"
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
