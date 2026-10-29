import React, { useState, useEffect } from "react";
import {
  GlassCard,
  Button,
  Badge,
  Switch,
  Input,
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
  Trash2,
  Plus,
} from "lucide-react";
import { api, type WorkforceOverviewDTO, type WorkforceModelPriceDTO } from "../../api";

// fmtPrice renders a per-million-token price without trailing zeros.
function fmtPrice(v: number): string {
  return `$${v.toFixed(4).replace(/\.?0+$/, "")}`;
}

export function BudgetsView() {
  const { t } = useTranslation();
  const [overview, setOverview] = useState<WorkforceOverviewDTO | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [prices, setPrices] = useState<WorkforceModelPriceDTO[]>([]);
  const [priceError, setPriceError] = useState<string | null>(null);
  const [priceModel, setPriceModel] = useState("");
  const [priceIn, setPriceIn] = useState("");
  const [priceOut, setPriceOut] = useState("");
  const [savingPrice, setSavingPrice] = useState(false);

  const loadPrices = async () => {
    try {
      setPrices(await api.workforceModelPrices());
      setPriceError(null);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.pricesLoadError", "Could not load the model prices.");
      setPriceError(msg);
      setPrices([]); // Fail-closed: no price shown, no price assumed
    }
  };

  const loadOverview = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.workforceOverview();
      setOverview(data);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.budgetsLoadError", "Failed to load budget metrics");
      setError(msg);
      setOverview(null); // Fail-closed
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadOverview();
    loadPrices();
  }, []);

  const handleSavePrice = async (e: React.FormEvent) => {
    e.preventDefault();
    const model = priceModel.trim();
    if (!model) {
      toast.error(t("agentHarness.modelRequired", "Enter a model name."));
      return;
    }
    const inputPerMTok = Number(priceIn);
    const outputPerMTok = Number(priceOut);
    if (
      priceIn === "" ||
      priceOut === "" ||
      !Number.isFinite(inputPerMTok) ||
      !Number.isFinite(outputPerMTok) ||
      inputPerMTok < 0 ||
      outputPerMTok < 0
    ) {
      toast.error(t("agentHarness.priceInvalid", "Enter zero or a positive number."));
      return;
    }

    setSavingPrice(true);
    try {
      await api.setWorkforceModelPrice({ model, inputPerMTok, outputPerMTok });
      setPriceModel("");
      setPriceIn("");
      setPriceOut("");
      toast.success(t("agentHarness.priceSaved", "Model price saved."));
      await loadPrices();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.priceSaveError", "Could not save the model price. Try again.");
      toast.error(msg);
    } finally {
      setSavingPrice(false);
    }
  };

  const handleDeletePrice = async (model: string) => {
    try {
      await api.deleteWorkforceModelPrice(model);
      toast.success(t("agentHarness.priceDeleted", "Model price deleted."));
      await loadPrices();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : t("agentHarness.priceDeleteError", "Could not delete the model price. Try again.");
      toast.error(msg);
    }
  };

  const handleToggleAutoHalt = async (val: boolean) => {
    if (!overview) return;
    const prev = overview;
    // Optimistic mutation
    setOverview({ ...overview, autoHalt: val });

    try {
      await api.toggleWorkforceAutoHalt(val);
      toast.success(
        val
          ? t("agentHarness.autoHaltOn", "Auto-halt is on. Agents pause at 100% of budget.")
          : t("agentHarness.autoHaltOff", "Auto-halt is off.")
      );
    } catch (err: unknown) {
      // Rollback on rejection
      setOverview(prev);
      const msg = err instanceof Error ? err.message : t("agentHarness.autoHaltUpdateFallback", "Could not change auto-halt. Try again.");
      toast.error(msg);
    }
  };

  if (error) {
    return (
      <div className="space-y-6" data-testid="budgets-view-error">
        <GlassCard className="p-8 text-center flex flex-col items-center justify-center space-y-3">
          <AlertTriangle className="h-8 w-8 text-danger-fg" />
          <div>
            <h4 className="text-sm font-bold text-foreground">{t("agentHarness.budgetsLoadError")}</h4>
            <p className="text-xs text-muted-foreground mt-1 max-w-md">{error}</p>
          </div>
          <Button variant="outline" size="sm" onClick={loadOverview}>
            {t("agentHarness.retryBtn")}
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
            <GlassCard key={i} className="p-5 h-36 animate-pulse bg-muted">
              <div />
            </GlassCard>
          ))}
        </div>
      </div>
    );
  }

  const currentSpend = overview.monthlySpend;
  const monthlyLimit = overview.monthlyLimit;
  const pct = monthlyLimit > 0 ? Math.min(100, Math.round((currentSpend / monthlyLimit) * 100)) : 0;
  const autoHalt = overview.autoHalt;
  const spendByAgent = overview.spendByAgent ?? [];
  const spendByProvider = overview.spendByProvider ?? [];
  // Budgets cover the calendar month in UTC, the same period the server
  // enforces. The countdown is derived from the clock, never hardcoded.
  const now = new Date();
  const nextResetAt = Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 1);
  const daysUntilReset = Math.max(0, Math.ceil((nextResetAt - now.getTime()) / 86_400_000));

  return (
    <div className="space-y-6" data-testid="budgets-view">
      {/* Overview Stat Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.monthlySpend", "Workspace Monthly Spend")}</span>
            <Coins className="h-4 w-4 text-accent-fg" />
          </div>
          <div className="text-2xl font-bold font-mono text-foreground">
            ${currentSpend.toFixed(2)}
          </div>
          <div className="text-[11px] text-muted-foreground">
            {t("agentHarness.budgetLimit", "Limit:")} <strong className="text-foreground">${monthlyLimit.toFixed(2)}</strong> ({pct}{t("agentHarness.consumedPct", "% allocated")})
          </div>
          <div className="h-2 w-full rounded-full bg-border overflow-hidden mt-1">
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
              {autoHalt
                ? t("agentHarness.failClosedProtected", "Fail-Closed Protected")
                : t("agentHarness.advisoryOnly", "Advisory Only")}
            </Badge>
          </div>
          <p className="text-[11px] text-muted-foreground leading-relaxed pt-1">
            {t("agentHarness.autoHaltLimit", "Automatically suspends autonomous executions when cycle budget is exhausted.")}
          </p>
        </GlassCard>

        <GlassCard className="p-5 space-y-2">
          <div className="text-xs text-muted-foreground flex items-center justify-between">
            <span>{t("agentHarness.billingCycle", "Billing Cycle")}</span>
            <TrendingUp className="h-4 w-4 text-accent-fg" />
          </div>
          <div className="text-lg font-bold text-foreground">
            {t("agentHarness.daysUntilReset", "Days until reset: {days}", { days: daysUntilReset })}
          </div>
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
            <Switch checked={autoHalt} onCheckedChange={handleToggleAutoHalt} aria-label={t("agentHarness.runawayGuard", "Runaway Spending Protection")} />
            <span className="text-xs font-semibold text-foreground">
              {autoHalt
                ? t("agentHarness.statusActive", "Active")
                : t("agentHarness.budgetDisabled", "Disabled")}
            </span>
          </div>
        </div>
      </GlassCard>

      {/* Spend Breakdown */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <GlassCard className="p-5 space-y-3">
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <BarChart3 className="h-4 w-4 text-accent-fg" />
            {t("agentHarness.spendByAgent", "Spend by Agent")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {spendByAgent.length === 0 && (
              <div className="py-4 text-center text-muted-foreground">
                {t("agentHarness.noSpendRecorded", "No agent has accrued spend yet.")}
              </div>
            )}
            {spendByAgent.map((row) => (
              <div key={row.name} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.name}</span>
                  <span className="font-mono text-muted-foreground">
                    ${row.spend.toFixed(2)} ({row.pct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-border overflow-hidden">
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
            <Cpu className="h-4 w-4 text-accent-fg" />
            {t("agentHarness.spendByProvider", "Spend by Model Provider")}
          </h4>
          <div className="space-y-3 pt-2 text-xs">
            {spendByProvider.length === 0 && (
              <div className="py-4 text-center text-muted-foreground">
                {t("agentHarness.noSpendRecorded", "No agent has accrued spend yet.")}
              </div>
            )}
            {spendByProvider.map((row) => (
              <div key={row.provider} className="space-y-1">
                <div className="flex items-center justify-between">
                  <span className="font-medium text-foreground">{row.provider}</span>
                  <span className="font-mono text-muted-foreground">
                    ${row.spend.toFixed(2)} ({row.pct}%)
                  </span>
                </div>
                <div className="h-1.5 w-full rounded-full bg-border overflow-hidden">
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

      {/* Model Prices: the price source the billing path uses. A model with
          no price cannot bill a turn. */}
      <GlassCard className="p-5 space-y-4">
        <div>
          <h4 className="text-sm font-bold text-foreground flex items-center gap-2">
            <Coins className="h-4 w-4 text-accent-fg" />
            {t("agentHarness.modelPrices", "Model Prices")}
          </h4>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t(
              "agentHarness.modelPricesDesc",
              "Set the price of each model the agents use. An agent turn is billed at these prices."
            )}
          </p>
        </div>

        {priceError && <div className="text-xs text-danger-fg">{priceError}</div>}

        <div className="space-y-2">
          {prices.length === 0 && (
            <div className="py-2 text-xs text-muted-foreground">
              {t("agentHarness.noPrices", "No model price is set. Agent turns are not billed.")}
            </div>
          )}
          {prices.map((price) => (
            <div
              key={price.model}
              className="flex items-center justify-between gap-3 rounded-lg border border-border bg-well px-3 py-2 text-xs"
            >
              <span className="font-mono font-medium text-foreground truncate">{price.model}</span>
              <div className="flex items-center gap-4 shrink-0">
                <span className="text-muted-foreground">
                  {t("agentHarness.inputPrice", "Input $ per 1M tokens")}:{" "}
                  <span className="font-mono text-foreground">{fmtPrice(price.inputPerMTok)}</span>
                </span>
                <span className="text-muted-foreground">
                  {t("agentHarness.outputPrice", "Output $ per 1M tokens")}:{" "}
                  <span className="font-mono text-foreground">{fmtPrice(price.outputPerMTok)}</span>
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  aria-label={t("agentHarness.deletePrice", "Delete price")}
                  onClick={() => handleDeletePrice(price.model)}
                >
                  <Trash2 className="h-3.5 w-3.5 text-danger-fg" />
                </Button>
              </div>
            </div>
          ))}
        </div>

        <form onSubmit={handleSavePrice} className="flex flex-col sm:flex-row sm:items-end gap-3">
          <div className="flex-1 space-y-1">
            <label className="text-[11px] font-semibold text-muted-foreground">
              {t("agentHarness.modelColumn", "Model")}
            </label>
            <Input
              value={priceModel}
              onChange={(e) => setPriceModel(e.target.value)}
              placeholder={t("agentHarness.modelPlaceholder", "claude-opus-4-8")}
            />
          </div>
          <div className="w-full sm:w-36 space-y-1">
            <label className="text-[11px] font-semibold text-muted-foreground">
              {t("agentHarness.inputPrice", "Input $ per 1M tokens")}
            </label>
            <Input
              type="number"
              min="0"
              step="any"
              value={priceIn}
              onChange={(e) => setPriceIn(e.target.value)}
              placeholder="0"
            />
          </div>
          <div className="w-full sm:w-36 space-y-1">
            <label className="text-[11px] font-semibold text-muted-foreground">
              {t("agentHarness.outputPrice", "Output $ per 1M tokens")}
            </label>
            <Input
              type="number"
              min="0"
              step="any"
              value={priceOut}
              onChange={(e) => setPriceOut(e.target.value)}
              placeholder="0"
            />
          </div>
          <Button type="submit" size="sm" disabled={savingPrice}>
            <Plus className="h-3.5 w-3.5 mr-1" />
            {t("agentHarness.savePrice", "Save Price")}
          </Button>
        </form>
      </GlassCard>
    </div>
  );
}
