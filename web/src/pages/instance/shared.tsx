// Shared hooks & helpers for the instance console pages (web/src/pages/instance).
// The console shell gates on isInstanceAdmin before any page mounts, so the
// settings hook here skips the tenant-shell isInstanceAdmin dance entirely.
import { useCallback, useEffect, useState } from "react";
import { api, ReadinessCheck } from "../../api";
import { type BadgeTone } from "../../ui";

export function useInstanceSettings() {
  const [s, setS] = useState<import("../../api").InstanceSettings | null>(null);
  const reload = useCallback(() => api.instanceSettings().then(setS), []);
  useEffect(() => { reload(); }, [reload]);
  return { s, reload };
}

// The wizard's steps ARE the readiness checks that carry a fix action — the
// server is the single source of truth for what a step is and whether it's
// done; this file only selects/sorts what the API already said.
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

// A fixable step needing work — drives the first-launch wizard entry and the
// rail's "Setup wizard" item. Derivation only, no parallel step list.
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
