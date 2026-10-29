/**
 * QuotationCanvas: Interactive sales quotation spreadsheet, discount editor, and CRM synchronizer.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React, { useState } from "react";
import { Button, Table, THead, TBody, TR, TH, TD } from "@octarq/plugin-sdk";
import { Plus, Trash2, Building2, Calculator, RefreshCw } from "lucide-react";
import { useTranslation } from "../../../i18n";
import type { QuotationLineItem } from "../../cards/QuotationCard";
import { CanvasApprovalGate } from "../CanvasApprovalGate";
import type { CardActionHandler } from "../../cards/registry";
import { useCopilotStore } from "../../store";

export interface QuotationCanvasProps {
  artifactId: string;
  content: {
    quote_id?: string;
    client_name?: string;
    company?: string;
    line_items?: QuotationLineItem[];
    subtotal_cents?: number;
    discount_cents?: number;
    total_cents?: number;
    crm_sync_payload?: Record<string, unknown>;
    approval_id?: string;
  };
  onAction: CardActionHandler;
}

export function QuotationCanvas({ artifactId, content, onAction }: QuotationCanvasProps) {
  const { t } = useTranslation();
  const [items, setItems] = useState<QuotationLineItem[]>(() => {
    if (content.line_items && content.line_items.length > 0) {
      return content.line_items;
    }
    return [
      { sku: "OCT-ELITE", description: "Octarq Pro Elite License (Annual)", quantity: 1, unit_price_cents: 2000000, total_cents: 2000000 },
      { sku: "OCT-TRAFFIC-10M", description: "High-Volume Redirect Pack (10M/mo)", quantity: 1, unit_price_cents: 450000, total_cents: 450000 },
      { sku: "OCT-SLA-9999", description: "Enterprise 99.99% Support SLA", quantity: 1, unit_price_cents: 250000, total_cents: 250000 },
    ];
  });

  const [discountPercent, setDiscountPercent] = useState<number>(() => {
    if (typeof (content as any).discount_percent === "number") {
      return (content as any).discount_percent;
    }
    return 8;
  });

  const computeTotals = (lineItems: QuotationLineItem[], discountPct: number) => {
    const subtotal = lineItems.reduce((sum, item) => {
      const qty = item.quantity ?? item.qty ?? 1;
      const unitPrice = item.unit_price_cents ?? (item.unit_price ? Math.round(item.unit_price * 100) : 0);
      return sum + (unitPrice * qty);
    }, 0);
    const discount = Math.round((subtotal * discountPct) / 100);
    const total = subtotal - discount;
    return { subtotal, discount, total };
  };

  const { subtotal: subtotalCents, discount: discountCents, total: totalCents } = computeTotals(items, discountPercent);

  const syncToStore = (nextItems: QuotationLineItem[], nextDiscountPercent: number) => {
    const { subtotal, discount, total } = computeTotals(nextItems, nextDiscountPercent);
    useCopilotStore.getState().updateArtifactContent(artifactId, (prev: any) => ({
      ...prev,
      line_items: nextItems,
      discount_percent: nextDiscountPercent,
      subtotal_cents: subtotal,
      discount_cents: discount,
      total_cents: total,
    }));
  };

  const handleUpdateItem = (index: number, field: keyof QuotationLineItem, value: unknown) => {
    const next = [...items];
    next[index] = { ...next[index], [field]: value };
    setItems(next);
    syncToStore(next, discountPercent);
  };

  const handleAddItem = () => {
    const newItem: QuotationLineItem = {
      sku: `SKU-${items.length + 1}`,
      description: "Custom Professional Service Item",
      quantity: 1,
      unit_price_cents: 100000,
      total_cents: 100000,
    };
    const next = [...items, newItem];
    setItems(next);
    syncToStore(next, discountPercent);
  };

  const handleDeleteItem = (index: number) => {
    if (items.length <= 1) return;
    const next = items.filter((_, i) => i !== index);
    setItems(next);
    syncToStore(next, discountPercent);
  };

  const handleDiscountChange = (newPercent: number) => {
    setDiscountPercent(newPercent);
    syncToStore(items, newPercent);
  };

  const formatCurrency = (cents: number) => {
    return (cents / 100).toLocaleString("en-US", { style: "currency", currency: "USD" });
  };

  const approvalId = String(content.approval_id ?? "").trim();

  return (
    <div className="flex h-full flex-col space-y-3 p-4 overflow-y-auto" data-testid="quotation-canvas">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border pb-3">
        <div>
          <h3 className="text-base font-bold text-foreground">
            {`${t("copilot.quotationCanvasTitle", "Price Quotation —")} ${content.client_name || "Acme Corporation"}`}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t("copilot.quotationCanvasDesc", "Adjust line item quantities, pricing tiers, and commercial discounts.")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="secondary"
            onClick={handleAddItem}
            data-testid="add-line-item-btn"
            className="h-8 text-xs flex items-center gap-1"
          >
            <Plus className="h-3.5 w-3.5" />
            <span>{t("copilot.quotationAddItem", "Add Item")}</span>
          </Button>
        </div>
      </div>

      {/* Spreadsheet Table */}
      <div className="flex-1 overflow-auto rounded-xl border border-border bg-card">
        <Table className="w-full text-left text-xs border-collapse">
          <THead>
            <TR className="border-b border-border bg-well font-semibold text-muted-foreground">
              <TH className="py-2 px-3">SKU</TH>
              <TH className="py-2 px-3">{t("copilot.quotationColDesc", "Description")}</TH>
              <TH className="py-2 px-3 w-20">{t("copilot.quotationColQty", "Quantity")}</TH>
              <TH className="py-2 px-3 w-32">{t("copilot.quotationColUnit", "Unit Price")}</TH>
              <TH className="py-2 px-3 w-32 text-right">{t("copilot.quotationColLineTotal", "Line Total")}</TH>
              <TH className="py-2 px-3 w-12 text-center">{t("copilot.quotationColAction", "Action")}</TH>
            </TR>
          </THead>
          <TBody className="divide-y divide-border/40">
            {items.map((item, idx) => {
              const qty = item.quantity ?? item.qty ?? 1;
              const unitPrice = item.unit_price_cents ?? (item.unit_price ? Math.round(item.unit_price * 100) : 0);
              const lineTotal = unitPrice * qty;

              return (
                <TR key={idx} className="hover:bg-surface-hover/30 transition-colors" data-testid="line-item-row">
                  <TD className="py-2 px-3">
                    <input
                      type="text"
                      aria-label="SKU"
                      value={item.sku || ""}
                      onChange={(e) => handleUpdateItem(idx, "sku", e.target.value)}
                      className="w-full rounded border border-border bg-background px-2 py-1 font-mono text-xs focus:border-primary focus:outline-none"
                    />
                  </TD>
                  <TD className="py-2 px-3">
                    <input
                      type="text"
                      aria-label={t("copilot.quotationColDesc", "Description")}
                      value={item.description}
                      onChange={(e) => handleUpdateItem(idx, "description", e.target.value)}
                      className="w-full rounded border border-border bg-background px-2 py-1 text-xs focus:border-primary focus:outline-none"
                    />
                  </TD>
                  <TD className="py-2 px-3">
                    <input
                      type="number"
                      aria-label={t("copilot.quotationColQty", "Quantity")}
                      min={1}
                      value={qty}
                      onChange={(e) => handleUpdateItem(idx, "quantity", Math.max(1, parseInt(e.target.value) || 1))}
                      className="w-full rounded border border-border bg-background px-2 py-1 font-mono text-xs text-center focus:border-primary focus:outline-none"
                    />
                  </TD>
                  <TD className="py-2 px-3">
                    <input
                      type="number"
                      aria-label={t("copilot.quotationColUnit", "Unit Price")}
                      step={50}
                      value={unitPrice / 100}
                      onChange={(e) => handleUpdateItem(idx, "unit_price_cents", Math.round((parseFloat(e.target.value) || 0) * 100))}
                      className="w-full rounded border border-border bg-background px-2 py-1 font-mono text-xs text-right focus:border-primary focus:outline-none"
                    />
                  </TD>
                  <TD className="py-2 px-3 font-mono font-bold text-foreground text-right">
                    {formatCurrency(lineTotal)}
                  </TD>
                  <TD className="py-2 px-3 text-center">
                    <button
                      onClick={() => handleDeleteItem(idx)}
                      disabled={items.length <= 1}
                      className="rounded p-1 text-muted-foreground hover:bg-danger-bg hover:text-danger-fg disabled:opacity-30"
                      aria-label={t("copilot.quotationRemoveItemAria", "Remove item")}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      </div>

      {/* Pricing Summary & Discount Slider */}
      <div className="grid grid-cols-2 gap-3 rounded-xl border border-border bg-well p-3 text-xs">
        <div className="space-y-2">
          <label className="font-semibold text-foreground flex items-center gap-1.5">
            <Calculator className="h-3.5 w-3.5 text-accent-fg" />
            <span>{`${t("copilot.quotationCommercialDiscount", "Commercial Discount:")} ${discountPercent}%`}</span>
          </label>
          <input
            type="range"
            min={0}
            max={30}
            step={1}
            value={discountPercent}
            onChange={(e) => handleDiscountChange(parseInt(e.target.value) || 0)}
            className="w-full cursor-pointer accent-primary"
            aria-label={t("copilot.quotationCommercialDiscountAria", "Commercial Discount Percentage")}
          />
          <div className="text-[11px] text-muted-foreground">
            {t("copilot.quotationDiscountHelp", "Applies negotiated partner volume discount to quotation total.")}
          </div>
        </div>

        <div className="space-y-1 text-right">
          <div className="flex justify-between text-muted-foreground">
            <span>{`${t("copilot.quotationSubtotal", "Subtotal")}:`}</span>
            <span className="font-mono">{formatCurrency(subtotalCents)}</span>
          </div>
          <div className="flex justify-between text-success-fg">
            <span>{`${t("copilot.quotationDiscount", "Discount")} (${discountPercent}%):`}</span>
            <span className="font-mono">-{formatCurrency(discountCents)}</span>
          </div>
          <div className="flex justify-between text-sm font-bold text-foreground pt-1 border-t border-border">
            <span>{t("copilot.quotationGrandTotal", "Grand Total:")}</span>
            <span className="font-mono" data-testid="grand-total">{formatCurrency(totalCents)}</span>
          </div>
        </div>
      </div>

      {/* Approval Gate */}
      <CanvasApprovalGate
        approvalId={approvalId}
        toolName="crm_update_deal_and_dispatch_quote"
        actionLabel="Approve & Dispatch Quotation"
        riskLevel="write"
        description={t("copilot.quotationCanvasGateDesc", "Send quotation to client and advance CRM stage. Confirm deal amount before submission.")}
        payload={{
          quote_id: content.quote_id || "QUOTE-2026-001",
          total_cents: totalCents,
          client_name: content.client_name || "Acme Corporation",
          item_count: items.length,
        }}
        onAction={onAction}
      />
    </div>
  );
}
