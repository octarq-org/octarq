import { z } from "zod";

export const HealthStatusSchema = z.enum([
  "ok",
  "warn",
  "error",
  "healthy",
  "degraded",
  "down",
]);
export type HealthStatus = z.infer<typeof HealthStatusSchema>;

export const ProviderHealthReportSchema = z.object({
  name: z.string().default(""),
  status: HealthStatusSchema.catch("ok"),
  message: z.string().optional().default(""),
  category: z.string().optional().default(""),
  unit: z.string().optional().default(""),
  metrics: z.record(z.string(), z.unknown()).optional().default({}),
});
export type ProviderHealthReport = z.infer<typeof ProviderHealthReportSchema>;

export const HealthReportSchema = z.object({
  overall: HealthStatusSchema.catch("ok"),
  providers: z.array(ProviderHealthReportSchema).default([]),
  checked_at: z.string().optional().default(""),
});
export type HealthReport = z.infer<typeof HealthReportSchema>;

export const defaultHealthReport: HealthReport = {
  overall: "ok",
  providers: [],
  checked_at: "",
};

export type NormalizedHealthStatus = "healthy" | "degraded" | "down";

export function normalizeHealthStatus(status?: string): NormalizedHealthStatus {
  switch (status) {
    case "ok":
    case "healthy":
      return "healthy";
    case "warn":
    case "degraded":
      return "degraded";
    case "error":
    case "down":
      return "down";
    default:
      return "healthy";
  }
}

export function formatBytes(bytes: number | unknown): string {
  if (typeof bytes !== "number" || isNaN(bytes) || bytes < 0) return "-";
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1);
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatDuration(seconds: number | unknown): string {
  if (typeof seconds !== "number" || isNaN(seconds) || seconds < 0) return "-";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);

  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}
