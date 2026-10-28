import React, { useState, useEffect } from "react";
import {
  GlassCard,
  Button,
  Badge,
  Switch,
  toast,
  useTranslation,
} from "@octarq/plugin-sdk";
import {
  Coins,
  ShieldCheck,
  TrendingUp,
  AlertTriangle,
  Cpu,
  BarChart3,
} from "lucide-react";
import { api, type WorkforceOverviewDTO } from "../../api";

export function BudgetsView() {
  const { t } = useTranslation();
  const [overview, setOverview] = useState<WorkforceOverviewDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadOverview = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.workforceOverview();
      setOverview(data);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load budget overview.";
      setError(msg);
      setOverview(null); // Fail-closed
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadOverview();
  }, []);

  const handleToggleAutoHalt = async (val: boolean) => {
    if (!overview) return;
    const prev = overview;
    // Optimistic mutation
    setOverview({ ...overview, autoHalt: val });

    try {
      await api.toggleWorkforceAutoHalt(val);
      toast.success(
        val
          ? "Auto-halt enabled: Agents will pause automatically at 100% budget."
          : "Auto-halt disabled: Over-budget warnings only."
      );
    } catch (err: unknown) {
      // Rollback on rejection
      setOverview(prev);
      const msg = err instanceof Error ? err.message : "Failed to update auto-halt setting.";
      toast.error(msg);
    }
  };

  if (error) {
    return (
      <div className="space-y-6" data-testid="budgets-view-error">
        <GlassCard className="p-8 text-center flex flex-col items-center justify-center space-y-3">
          <AlertTriangle className="h-8 w-8 text-danger-fg" />
          <div>
            <h4 className="text-sm font-bold text-foreground">Failed to load budget metrics</h4>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">{error}</p>
          </div>
          <Button variant="outline" size="sm" onClick={loadOverview}>
            Retry
          </Button>
        </GlassCard>
      </div>
    );
  }

  if (loading || !overview) {
    return (
      <div className="space-y-6" data-testid="budgets-view">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {[1, 2, 3].map((i) => (
            <GlassCard key={i} className="p-5 h-36 animate-pulse bg-muted/20">
              <div />
            </GlassCard>
          ))}
        </div>
      </div>
    );
  }

  const currentSpend = overview.monthlySpend;
  const monthlyLimit = overview.monthlyLimit > 0 ? overview.monthlyLimit : 50.0;
  const pct = Math.min(100, Math.round((currentSpend / monthlyLimit) * 100));
  const autoHalt = overview.autoHalt;

  return (
    <div className="space-y-6" data-testid="budgets-view">
      {/* Overview Stat Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.monthlySpend", "Workspace Monthly Spend")}</span>
            <Coins className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold font-mono text-foreground">
            ${currentSpend.toFixed(2)}
          </div>
          <div className="text-[11px] text-muted-foreground">
            {t("agentHarness.budgetLimit", "Limit:")} <strong className="text-foreground">${monthlyLimit.toFixed(2)}</strong> ({pct}{t("agentHarness.consumedPct", "% allocated")})
          </div>
          <div className="h-2 w-full rounded-full bg-foreground/10 overflow-hidden mt-1">
            <div
              className={`h-full rounded-full transition-all duration-300 ${
                pct > 80 ? "bg-warning-fg" : "bg-primary"
              }`}
              style={{ width: `${pct}%` }}
            />
          </div>
        </GlassCard>

        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.hardStopGov", "Hard-Stop Governance")}</span>
            <ShieldCheck className="h-4 w-4 text-success-fg" />
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Badge variant={autoHalt ? "success" : "warning"} className="text-xs">
              {autoHalt ? "Fail-Closed Protected" : "Advisory Only"}
            </Badge>
          </div>
          <p className="text-[11px] text-muted-foreground leading-relaxed pt-1">
            {t("agentHarness.autoHaltLimit", "Automatically suspends autonomous executions when cycle budget is exhausted.")}
          </p>
        </GlassCard>

        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.billingCycle", "Billing Cycle")}</span>
            <TrendingUp className="h-4 w-4 text-primary" />
          </div>
          <div className="text-lg font-bold text-foreground">{t("agentHarness.nextReset", "Renews in 12 days")}</div>
          <div className="text-[11px] text-muted-foreground">
            {t("agentHarness.cycleResetDesc", "Agent budgets reset automatically at UTC 00:00 on the 1st.")}
          </div>
        </GlassCard>
      </div>

      {/* Auto-Halt Guard Configuration */}
      <GlassCard className="p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <h4 className="text-sm font-bold text-foreground">
              {t("agentHarness.runawayGuard", "Runaway Spending Protection")}
            </h4>
            <p className="text-xs text-muted-foreground mt-0.5">
              {t("agentHarness.runawayGuardDesc", "When enabled, agents enter paused state if total workspace spend hits limit.")}
            </p>
          </div>
          <div className="flex items-center gap-3">
            <Switch checked={autoHalt} onCheckedChange={handleToggleAutoHalt} />
            <span className="text-xs font-semibold text-foreground">
              {autoHalt ? "Active" : "Disabled"}
            </span>
          </div>
        </div>
      </GlassCard>

      {/* Spend Breakdown */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <GlassCard className="p-5 space-y-3">
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <BarChart3 className="h-4 w-4 text-primary" />
            {t("agentHarness.spendByAgent", "Spend by Agent")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {(overview.spendByAgent || [
              { name: "Lead Orchestrator", model: "GPT-4o", spend: 5.80, pct: 41 },
              { name: "DevOps Specialist", model: "Claude 3.5 Sonnet", spend: 4.50, pct: 32 },
              { name: "Security Auditor", model: "GPT-4o", spend: 2.10, pct: 15 },
              { name: "Mail Deliverability Agent", model: "DeepSeek-V3", spend: 1.80, pct: 12 },
            ]).map((row) => (
              <div key={row.name} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.name}</span>
                  <span className="font-mono text-muted-foreground">
                    ${row.spend.toFixed(2)} ({row.pct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-foreground/10 overflow-hidden">
                  <div
                    className="h-full rounded-full bg-primary"
                    style={{ width: `${row.pct}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </GlassCard>

        <GlassCard className="p-5 space-y-3">
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <Cpu className="h-4 w-4 text-primary" />
            {t("agentHarness.spendByProvider", "Spend by Model Provider")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {(overview.spendByProvider || [
              { provider: "OpenAI (GPT-4o)", spend: 7.90, pct: 56 },
              { provider: "Anthropic (Claude 3.5 Sonnet)", spend: 4.50, pct: 32 },
              { provider: "DeepSeek (DeepSeek-V3)", spend: 1.80, pct: 12 },
            ]).map((row) => (
              <div key={row.provider} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.provider}</span>
                  <span className="font-mono text-muted-foreground">
                    ${row.spend.toFixed(2)} ({row.pct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-foreground/10 overflow-hidden">
                  <div
                    className="h-full rounded-full bg-success-fg"
                    style={{ width: `${row.pct}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        </GlassCard>
      </div>
    </div>
  );
}
