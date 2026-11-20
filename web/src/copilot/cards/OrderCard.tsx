import React from "react";
import { ShoppingCart } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";

export interface OrderArgs {
  orderNo?: string;
  limit?: number;
}

export function OrderCard({
  args,
  toolName,
  state,
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const a = (args ?? {}) as OrderArgs;
  const orderNo = (a.orderNo ?? "").trim();
  const isList = toolName === "orders_list";

  return (
    <CardShell title={isList ? t("copilot.cardOrdersList", "Orders — List") : `${t("copilot.cardOrderLookup", "Order — Lookup")}${orderNo ? ` (${orderNo})` : ""}`} state={state}>
      <div className="flex items-start gap-2">
        <ShoppingCart className="h-4 w-4 text-accent-fg shrink-0 mt-0.5" />
        <div className="flex-1 space-y-1">
          {isList ? (
            <div data-testid="order-list" className="font-mono text-xs text-muted-foreground">
              {t("copilot.cardOrdersListDesc", "Requested latest orders. Open Commerce to inspect full rows.")}
            </div>
          ) : (
            <div data-testid="order-no" className="font-mono text-xs">
              <span className="text-muted-foreground">
                <span>{t("copilot.cardOrderPrefix", "Order:")} </span>
              </span>
              <span className="font-semibold text-foreground">{orderNo}</span>
            </div>
          )}
          <div className="text-[11px] text-muted-foreground">
            {t("copilot.cardOrdersHint", "Complete details display under Commerce → Orders.")}
          </div>
        </div>
      </div>
    </CardShell>
  );
}
