/**
 * FinanceCard: Ledger balance and subscription queries.
 * Read-only display of financial query parameters.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { DollarSign } from "lucide-react";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface FinanceArgs {
  currency?: string;
  category?: string;
  kind?: string;
  limit?: number;
}

function MetricRow({ label, value, testid }: { label: string; value: string; testid: string }) {
  return (
    <div className="flex items-baseline justify-between gap-2 font-mono text-xs" data-testid={testid}>
      <span className="text-muted-foreground">{label}</span>
      <span className="font-semibold text-foreground">{value}</span>
    </div>
  );
}

export function FinanceCard({
  args,
  toolName,
  state,
}: ToolCardProps & { state?: CardState }) {
  const a = (args ?? {}) as FinanceArgs;

  return (
    <CardShell title={`Finance — ${toolName.replace("finance_", "")}`} state={state}>
      <div className="space-y-1.5">
        {a.currency && <MetricRow label="Currency" value={a.currency} testid="finance-currency" />}
        {a.kind && <MetricRow label="Kind" value={a.kind} testid="finance-kind" />}
        {a.category && <MetricRow label="Category" value={a.category} testid="finance-category" />}
        {a.limit !== undefined && <MetricRow label="Limit" value={String(a.limit)} testid="finance-limit" />}
        {!a.currency && !a.kind && !a.category && a.limit === undefined && (
          <div className="font-mono text-xs text-muted-foreground">
            Subscription review requested. Full ledger lives under Finance.
          </div>
        )}
      </div>
    </CardShell>
  );
}
