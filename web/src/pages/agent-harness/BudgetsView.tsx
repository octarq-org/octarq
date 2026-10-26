import React, { useState } from "react";
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
  Sliders,
} from "lucide-react";

export function BudgetsView() {
  const { t } = useTranslation();
  const [autoHalt, setAutoHalt] = useState(true);
  const [monthlyLimit, setMonthlyLimit] = useState(50.0);
  const currentSpend = 14.2;
  const pct = Math.min(100, Math.round((currentSpend / monthlyLimit) * 100));

  const handleToggleAutoHalt = (val: boolean) => {
    setAutoHalt(val);
    toast.success(
      val
        ? "Auto-halt enabled: Agents will pause automatically at 100% budget."
        : "Auto-halt disabled: Over-budget warnings only."
    );
  };

  return (
    <div className="space-y-6">
      {/* Overview Stat Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.monthlySpend")}</span>
            <Coins className="h-4 w-4 text-primary" />
          </div>
          <div className="text-2xl font-bold font-mono text-foreground">
            ${currentSpend.toFixed(2)}
          </div>
          <div className="text-[11px] text-muted-foreground">
            {t("agentHarness.budgetLimit")} <strong className="text-foreground">${monthlyLimit.toFixed(2)}</strong> ({pct}{t("agentHarness.consumedPct")})
          </div>
          <div className="h-2 w-full rounded-full bg-foreground/10 overflow-hidden mt-1">
            <div
              className={`h-full rounded-full transition-all duration-300 ${
                pct > 80 ? "bg-amber-500" : "bg-primary"
              }`}
              style={{ width: `${pct}%` }}
            />
          </div>
        </GlassCard>

        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.hardStopGov")}</span>
            <ShieldCheck className="h-4 w-4 text-emerald-500" />
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Badge variant={autoHalt ? "green" : "amber"} className="text-xs">
              {autoHalt ? "Fail-Closed Protected" : "Advisory Only"}
            </Badge>
          </div>
          <p className="text-[11px] text-muted-foreground leading-relaxed pt-1">
            {t("agentHarness.autoHaltLimit")}
          </p>
        </GlassCard>

        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.billingCycle")}</span>
            <TrendingUp className="h-4 w-4 text-blue-400" />
          </div>
          <div className="text-lg font-bold text-foreground">{t("agentHarness.nextReset")}</div>
          <div className="text-[11px] text-muted-foreground">
            {t("agentHarness.cycleResetDesc")}
          </div>
        </GlassCard>
      </div>

      {/* Auto-Halt Guard Configuration */}
      <GlassCard className="p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <h4 className="text-sm font-bold text-foreground">
              {t("agentHarness.runawayGuard")}
            </h4>
            <p className="text-xs text-muted-foreground mt-0.5">
              {t("agentHarness.runawayGuardDesc")}
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

      {/* Spend by Agent & Model */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <GlassCard className="p-5 space-y-3">
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <BarChart3 className="h-4 w-4 text-primary" />
            {t("agentHarness.spendByAgent")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {[
              { name: "Lead Orchestrator", model: "GPT-4o", spend: "$5.80", pct: 41 },
              { name: "DevOps Specialist", model: "Claude 3.5 Sonnet", spend: "$4.50", pct: 32 },
              { name: "Security Auditor", model: "GPT-4o", spend: "$2.10", pct: 15 },
              { name: "Mail Deliverability Agent", model: "DeepSeek-V3", spend: "$1.80", pct: 12 },
            ].map((row) => (
              <div key={row.name} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.name}</span>
                  <span className="font-mono text-muted-foreground">
                    {row.spend} ({row.pct}%)
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
            {t("agentHarness.spendByProvider")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {[
              { provider: "OpenAI (GPT-4o)", spend: "$7.90", pct: 56 },
              { provider: "Anthropic (Claude 3.5 Sonnet)", spend: "$4.50", pct: 32 },
              { provider: "DeepSeek (DeepSeek-V3)", spend: "$1.80", pct: 12 },
            ].map((row) => (
              <div key={row.provider} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.provider}</span>
                  <span className="font-mono text-muted-foreground">
                    {row.spend} ({row.pct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-foreground/10 overflow-hidden">
                  <div
                    className="h-full rounded-full bg-emerald-500"
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
