import { GlassCard, Badge } from "@octarq/plugin-sdk";
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Database,
  HardDrive,
} from "lucide-react";
import { useTranslation } from "../../../i18n";
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

export function StatusIcon({ status }: { status: NormalizedHealthStatus }) {
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

export function RuntimeCard({ provider }: { provider: ProviderHealthReport }) {
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

export function DatabaseCard({ provider }: { provider: ProviderHealthReport }) {
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

export function DiskCard({ provider }: { provider: ProviderHealthReport }) {
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

export function GenericProviderCard({ provider }: { provider: ProviderHealthReport }) {
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
