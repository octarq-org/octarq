import { useQuery } from "@tanstack/react-query";
import { req, ApiError } from "../../api";
import { parseWithFallback } from "../../lib/parseWithFallback";
import { defaultHealthReport, HealthReport, HealthReportSchema } from "./types";

export async function fetchHealthReport(): Promise<HealthReport> {
  try {
    const raw = await req<unknown>("GET", "/api/monitor/health");
    return parseWithFallback(HealthReportSchema, raw, defaultHealthReport);
  } catch (err) {
    if (err instanceof ApiError && err.status === 503 && err.body) {
      return parseWithFallback(HealthReportSchema, err.body, defaultHealthReport);
    }
    throw err;
  }
}

export const HEALTH_QUERY_KEY = ["monitor", "health"] as const;

export function useHealthQuery(options?: { refetchInterval?: number | false; enabled?: boolean }) {
  return useQuery({
    queryKey: HEALTH_QUERY_KEY,
    queryFn: fetchHealthReport,
    refetchInterval: options?.refetchInterval ?? 30000,
    refetchIntervalInBackground: false,
    enabled: options?.enabled ?? true,
  });
}
