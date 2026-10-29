/**
 * QuotationCard: Sales price quotation and CRM synchronization card.
 *
 * All user copy strictly conforms to ASD-STE100 (<= 20 words/sentence, active voice).
 */
import React from "react";
import { Button } from "@octarq/plugin-sdk";
import { Receipt, Building2, Send, ExternalLink, ShieldAlert } from "lucide-react";
import { useTranslation } from "../../i18n";
import type { ToolCardProps } from "./registry";
import { CardShell, type CardState } from "./CardShell";
import { useCopilotStore, type Artifact } from "../store";

export interface QuotationLineItem {
  sku?: string;
  description: string;
  quantity?: number;
  qty?: number;
  unit_price_cents?: number;
  unit_price?: number;
  total_cents?: number;
  total?: number;
}

export interface QuotationArgs {
  quote_id?: string;
  client_name?: string;
  customer_name?: string;
  company?: string;
  currency?: string;
  discount_percent?: number;
  discount_cents?: number;
  subtotal_cents?: number;
  total_cents?: number;
  grand_total?: number;
  valid_until?: string;
  line_items?: QuotationLineItem[];
  items?: QuotationLineItem[];
  crm_sync_payload?: Record<string, unknown>;
  crm_payload?: Record<string, unknown>;
  approval_id?: string;
  approvalId?: string;
  state?: CardState;
  errorDetail?: string;
  forbiddenReason?: string;
}

export function QuotationCard({
  args,
  onAction,
  state: propState,
  toolName = "quotation_generate",
}: ToolCardProps & { state?: CardState }) {
  const { t } = useTranslation();
  const data = (args ?? {}) as QuotationArgs;
  const effectiveState = propState ?? data.state;
  const errorDetail = data.errorDetail;
  const forbiddenReason = data.forbiddenReason;

  const clientName = data.client_name ?? data.customer_name ?? "Acme Corporation";
  const company = data.company ?? "Acme Cloud Logistics";
  const quoteId = data.quote_id ?? "QUOTE-2026-001";
  const items = Array.isArray(data.line_items)
    ? data.line_items
    : Array.isArray(data.items)
      ? data.items
      : [];

  const subtotalCents = data.subtotal_cents ?? 2700000;
  const discountCents = data.discount_cents ?? 200000;
  const totalCents = data.total_cents ?? (data.grand_total ? Math.round(data.grand_total * 100) : subtotalCents - discountCents);
  const approvalId = String(data.approval_id ?? data.approvalId ?? "").trim();

  const artifactId = `artifact-quotation-${quoteId.toLowerCase()}`;

  const formatCurrency = (cents: number) => {
    return (cents / 100).toLocaleString("en-US", { style: "currency", currency: "USD" });
  };

  const handleOpenCanvas = () => {
    const artifact: Artifact = {
      id: artifactId,
      type: "quotation",
      title: `Quotation — ${clientName}`,
      version: 1,
      status: "reviewing",
      content: {
        quote_id: quoteId,
        client_name: clientName,
        company,
        line_items: items,
        subtotal_cents: subtotalCents,
        discount_cents: discountCents,
        total_cents: totalCents,
        crm_sync_payload: data.crm_sync_payload || data.crm_payload || {},
        approval_id: approvalId,
      },
      mutatingAction: {
        toolName: "crm_update_deal_and_dispatch_quote",
        approvalId,
        riskLevel: "write",
        payload: {
          quote_id: quoteId,
          total_cents: totalCents,
          client_name: clientName,
        },
      },
      createdAt: new Date().toISOString(),
    };

    useCopilotStore.getState().openArtifact(artifact);
    void onAction({
      type: "open_canvas",
      toolName,
      artifactId,
      payload: artifact.content,
    });
  };

  const handleSendQuotation = () => {
    if (!approvalId) return;
    void onAction({
      type: "approve",
      toolName: "crm_update_deal_and_dispatch_quote",
      approvalId,
      payload: {
        quote_id: quoteId,
        total_cents: totalCents,
      },
    });
  };

  return (
    <CardShell
      title={t("copilot.quotationCardTitle", "Sales Price Quotation")}
      state={effectiveState}
      errorDetail={errorDetail}
      forbiddenReason={forbiddenReason}
    >
      <div className="space-y-2.5">
        <div className="flex items-center justify-between border-b border-border pb-1.5">
          <div className="flex items-center gap-1.5">
            <Building2 className="h-3.5 w-3.5 text-muted-foreground" />
            <div>
              <div className="font-semibold text-foreground text-xs" data-testid="client-name">
                {clientName}
              </div>
              <div className="text-[11px] text-muted-foreground">{company}</div>
            </div>
          </div>
          <div className="text-right">
            <div className="font-mono text-xs font-bold text-foreground" data-testid="total-amount">
              {formatCurrency(totalCents)}
            </div>
            <div className="text-[10px] text-muted-foreground font-mono">{quoteId}</div>
          </div>
        </div>

        {items.length === 0 ? (
          <div className="text-xs text-muted-foreground py-2 text-center" data-testid="empty-items">
            {t("copilot.quotationNoItems", "No line items to display.")}
          </div>
        ) : (
          <div className="space-y-1" data-testid="line-items-preview">
            {items.slice(0, 3).map((item, idx) => {
              const qty = item.quantity ?? item.qty ?? 1;
              const price = item.total_cents ?? (item.total ? Math.round(item.total * 100) : 0);
              return (
                <div
                  key={item.sku || idx}
                  className="flex items-center justify-between rounded-lg border border-border bg-well px-2.5 py-1 text-xs"
                >
                  <div className="truncate mr-2">
                    <span className="font-medium text-foreground">{item.description}</span>
                    <span className="text-[11px] text-muted-foreground ml-1.5">{`x${qty}`}</span>
                  </div>
                  <span className="font-mono text-[11px] font-semibold text-foreground shrink-0">
                    {price > 0 ? formatCurrency(price) : "—"}
                  </span>
                </div>
              );
            })}
          </div>
        )}

        {!approvalId && (
          <div
            role="alert"
            data-testid="approval-missing-id"
            className="flex items-center gap-1.5 rounded-lg border border-warning-fg/30 bg-warning-fg/10 p-2 text-[11px] text-warning-fg"
          >
            <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
            <span>{t("copilot.cardMissingApprovalId", "No approval ID provided. You cannot submit this action.")}</span>
          </div>
        )}

        <div className="flex items-center justify-between gap-2 pt-1 border-t border-border">
          <Button
            size="sm"
            variant="secondary"
            onClick={handleOpenCanvas}
            data-testid="open-canvas-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <ExternalLink className="h-3 w-3" />
            <span>{t("copilot.cardOpenInCanvas", "Open in Canvas")}</span>
          </Button>

          <Button
            size="sm"
            variant="primary"
            disabled={!approvalId}
            onClick={handleSendQuotation}
            data-testid="send-quotation-btn"
            className="h-7 text-xs flex items-center gap-1"
          >
            <Send className="h-3 w-3" />
            <span>{t("copilot.cardSendQuotation", "Send Quotation")}</span>
          </Button>
        </div>
      </div>
    </CardShell>
  );
}
