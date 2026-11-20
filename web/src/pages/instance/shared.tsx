import { type BadgeTone } from "@octarq/plugin-sdk";
import { useCallback, useEffect, useState } from "react";
import { api, ReadinessCheck } from "../../api";

export function useInstanceSettings() {
  const [s, setS] = useState<import("../../api").InstanceSettings | null>(null);
  const reload = useCallback(() => api.instanceSettings().then(setS), []);
  useEffect(() => { reload(); }, [reload]);
  return { s, reload };
}

export function useInstanceReadiness() {
  const [checks, setChecks] = useState<ReadinessCheck[] | null>(null);
  const [failed, setFailed] = useState(false);
  const reload = useCallback(() => {
    api.instanceReadiness()
      .then((c) => { setChecks(c); setFailed(false); })
      .catch(() => { setChecks(null); setFailed(true); });
  }, []);
  useEffect(() => { reload(); }, [reload]);
  return { checks, failed, reload };
}

export function fixableChecks(checks: ReadinessCheck[]): ReadinessCheck[] {
  return checks.filter((c) => !!c.fixPath);
}

export function hasFixableIssues(checks: ReadinessCheck[] | null): boolean {
  if (!checks) return false;
  return fixableChecks(checks).some((c) => c.status !== "ok");
}

export function stepState(status: ReadinessCheck["status"]): "blocked" | "degraded" | "ok" {
  if (status === "blocked") return "blocked";
  if (status === "degraded") return "degraded";
  return "ok";
}

export interface StepBadge {
  key: string;
  tone: BadgeTone;
}
export function stepBadge(status: ReadinessCheck["status"]): StepBadge {
  if (status === "blocked") return { key: "instance.stepBlocked", tone: "danger" };
  if (status === "degraded") return { key: "instance.stepDegraded", tone: "warning" };
  return { key: "instance.stepDone", tone: "green" };
}
