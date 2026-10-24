/**
 * OrderCard: Commerce order display.
 * Read-only display of requested order number or list parameters.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { ShoppingCart } from "lucide-react";
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
  const a = (args ?? {}) as OrderArgs;
  const orderNo = (a.orderNo ?? "").trim();
  const isList = toolName === "orders_list";

  return (
    <CardShell title={isList ? "Orders — List" : `Order — ${orderNo || "Lookup"}`} state={state}>
      <div className="flex items-start gap-2">
        <ShoppingCart className="h-4 w-4 text-primary shrink-0 mt-0.5" />
        <div className="flex-1 space-y-1">
          {isList ? (
            <div data-testid="order-list" className="font-mono text-xs text-muted-foreground">
              Requested latest{a.limit ? ` ${a.limit}` : ""} orders. Open Commerce to inspect full rows.
            </div>
          ) : (
            <div data-testid="order-no" className="font-mono text-xs">
              <span className="text-muted-foreground">Order: </span>
              <span className="font-semibold text-foreground">{orderNo}</span>
            </div>
          )}
          <div className="text-[11px] text-muted-foreground">
            Complete details display under Commerce → Orders.
          </div>
        </div>
      </div>
    </CardShell>
  );
}
