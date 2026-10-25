/**
 * Card registry: tool name to component mapping.
 *
 * - registerCard throws on duplicates (Fail-Closed, preventing duplicate ambiguity).
 * - resolveCard returns null for unregistered tools.
 * - renderToolCall validates args against schema before component sees them.
 * - Components receive valid args or FallbackCard renders with reason.
 */
import React, { type ComponentType, type ReactNode } from "react";
import { validateArgs, type JsonSchema } from "./validate";
import { FallbackCard } from "./FallbackCard";
import { ApprovalCard } from "./ApprovalCard";
import { LinkCard } from "./LinkCard";
import { EmployeeCard } from "./EmployeeCard";
import { OrderCard } from "./OrderCard";
import { FinanceCard } from "./FinanceCard";
import { StatusCard } from "./StatusCard";
import { PortalCard } from "./PortalCard";
import { ResumeRankCard } from "./ResumeRankCard";
import { InvoiceAuditCard } from "./InvoiceAuditCard";
import { QuotationCard } from "./QuotationCard";
import { WeeklyReportCard } from "./WeeklyReportCard";
import { ScriptStoryboardCard } from "./ScriptStoryboardCard";
import { EmailManagementCard } from "./EmailManagementCard";
import { SupportEscalationCard } from "./SupportEscalationCard";
import { FinancialAnalysisCard } from "./FinancialAnalysisCard";

export interface CardAction {
  /** approve/reject feed approval endpoints; custom/open_canvas/edit_artifact is domain-specific. */
  type: "approve" | "reject" | "custom" | "open_canvas" | "edit_artifact" | "refresh";
  approvalId?: string;
  approvalToken?: string;
  toolName?: string;
  artifactId?: string;
  payload?: unknown;
}

export interface CardOutcome {
  status: number;
}

export type CardActionHandler = (action: CardAction) => void | Promise<CardOutcome>;

export interface ToolCardProps {
  args: unknown;
  callId?: string;
  toolName: string;
  onAction: CardActionHandler;
}

export type ToolCardComponent = ComponentType<ToolCardProps>;

const cardRegistry = new Map<string, ToolCardComponent>();
const schemaRegistry = new Map<string, JsonSchema>();

export function registerCard(
  toolName: string,
  component: ToolCardComponent,
): void {
  if (cardRegistry.has(toolName)) {
    throw new Error(`card already registered: ${toolName}`);
  }
  cardRegistry.set(toolName, component);
}

export function resolveCard(toolName: string): ToolCardComponent | null {
  return cardRegistry.get(toolName) ?? null;
}

export function registerSchema(toolName: string, schema: JsonSchema): void {
  schemaRegistry.set(toolName, schema);
}

export function schemasFor(toolName: string): JsonSchema | null {
  return schemaRegistry.get(toolName) ?? null;
}

export function clearCards(): void {
  cardRegistry.clear();
  schemaRegistry.clear();
}

/** carriesApprovalID reports whether parsed args name an approval. */
export function carriesApprovalID(args: unknown): boolean {
  if (typeof args !== "object" || args === null || Array.isArray(args)) {
    return false;
  }
  const record = args as Record<string, unknown>;
  for (const key of ["approval_id", "approvalId", "_approval_id"]) {
    const v = record[key];
    if (typeof v === "string" && v.trim() !== "") return true;
  }
  return false;
}

export const WORKFLOW_TOOLS = new Set([
  "recruiting_screen",
  "invoice_audit",
  "quotation_generate",
  "sales_quotation",
  "weekly_report",
  "operations_weekly_report",
  "script_storyboard",
  "marketing_storyboard",
  "email_manage",
  "mail_draft_create",
  "create_email_draft",
  "mail_send_reply",
  "send_email",
  "support_escalate",
  "financial_analysis",
]);

/**
 * Render one tool call: resolve -> parse -> schema-check -> component.
 * Every failure branch renders FallbackCard with a machine-readable reason.
 * Never throws and never returns blank.
 */
export function renderToolCall(
  toolName: string,
  callId: string,
  args: unknown,
  onAction: CardActionHandler,
): ReactNode {
  let parsedArgs: unknown = args;

  if (typeof args === "string") {
    try {
      parsedArgs = JSON.parse(args);
    } catch {
      return (
        <FallbackCard
          toolName={toolName}
          callId={callId}
          reason="malformed-args"
          onAction={onAction}
        />
      );
    }
  }

  // Any call carrying an approval ID renders ApprovalCard (unless handled by a dedicated workflow card)
  if (carriesApprovalID(parsedArgs) && !WORKFLOW_TOOLS.has(toolName)) {
    return (
      <ApprovalCard
        args={parsedArgs}
        callId={callId}
        toolName={toolName}
        onAction={onAction}
      />
    );
  }

  const Component = resolveCard(toolName);
  if (!Component) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="unregistered"
        onAction={onAction}
      />
    );
  }

  const schema = schemasFor(toolName);
  if (!schema) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="no-schema"
        onAction={onAction}
      />
    );
  }

  const errors = validateArgs(schema, parsedArgs);
  if (errors.length > 0) {
    return (
      <FallbackCard
        toolName={toolName}
        callId={callId}
        reason="invalid-args"
        detail={errors[0]}
        onAction={onAction}
      />
    );
  }

  return (
    <Component
      args={parsedArgs}
      callId={callId}
      toolName={toolName}
      onAction={onAction}
    />
  );
}

const DEFAULT_SCHEMAS: Record<string, JsonSchema> = {
  links_create: {
    type: "object",
    required: ["slug", "target"],
    properties: {
      host: { type: "string" },
      slug: { type: "string" },
      target: { type: "string" },
      enabled: { type: "boolean" },
    },
  },
  links_update: {
    type: "object",
    properties: {
      host: { type: "string" },
      slug: { type: "string" },
      target: { type: "string" },
      enabled: { type: "boolean" },
    },
  },
  links_get: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  links_list: {
    type: "object",
    properties: {
      limit: { type: "integer" },
    },
  },
  links_disable: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  links_batch_disable: {
    type: "object",
    properties: {},
  },
  crew_create: {
    type: "object",
    properties: {
      name: { type: "string" },
      soul: { type: "string" },
    },
  },
  crew_get: {
    type: "object",
    properties: {
      id: { type: "number" },
    },
  },
  crew_list: {
    type: "object",
    properties: {},
  },
  crew_update_soul: {
    type: "object",
    properties: {
      id: { type: "number" },
      soul: { type: "string" },
    },
  },
  orders_get: {
    type: "object",
    properties: {
      orderNo: { type: "string" },
    },
  },
  orders_list: {
    type: "object",
    properties: {
      limit: { type: "integer" },
    },
  },
  finance_balance: {
    type: "object",
    properties: {
      currency: { type: "string" },
    },
  },
  finance_list_entries: {
    type: "object",
    properties: {
      currency: { type: "string" },
      category: { type: "string" },
      kind: { type: "string" },
      limit: { type: "integer" },
    },
  },
  finance_list_subscriptions: {
    type: "object",
    properties: {},
  },
  statuspage_public: {
    type: "object",
    properties: {
      slug: { type: "string" },
    },
  },
  statuspage_probe: {
    type: "object",
    properties: {
      target: { type: "string" },
    },
  },
  portal_licenses: {
    type: "object",
    properties: {
      email: { type: "string" },
    },
  },
  portal_devices: {
    type: "object",
    properties: {
      email: { type: "string" },
      licenseId: { type: "number" },
    },
  },
  recruiting_screen: {
    type: "object",
    properties: {
      requisition_id: { type: "string" },
      min_score: { type: "number" },
      job_title: { type: "string" },
      total_candidates: { type: "integer" },
      shortlist_count: { type: "integer" },
      candidates: { type: "array" },
      approval_id: { type: "string" },
    },
  },
  invoice_audit: {
    type: "object",
    properties: {
      batch_id: { type: "string" },
      audit_quarter: { type: "string" },
      audit_time_seconds: { type: "number" },
      total_invoices: { type: "integer" },
      scanned_count: { type: "integer" },
      matched_count: { type: "integer" },
      discrepancy_count: { type: "integer" },
      mismatched_count: { type: "integer" },
      overdue_count: { type: "integer" },
      net_variance_cents: { type: "integer" },
      total_variance_usd: { type: "number" },
      discrepancies: { type: "array" },
      items: { type: "array" },
      approval_id: { type: "string" },
    },
  },
  quotation_generate: {
    type: "object",
    properties: {
      transcript_id: { type: "string" },
      quote_id: { type: "string" },
      client_name: { type: "string" },
      customer_name: { type: "string" },
      company: { type: "string" },
      currency: { type: "string" },
      discount_percent: { type: "number" },
      discount_cents: { type: "integer" },
      subtotal_cents: { type: "integer" },
      total_cents: { type: "integer" },
      grand_total: { type: "number" },
      valid_until: { type: "string" },
      line_items: { type: "array" },
      items: { type: "array" },
      crm_sync_payload: { type: "object" },
      crm_payload: { type: "object" },
      approval_id: { type: "string" },
    },
  },
  sales_quotation: {
    type: "object",
    properties: {
      transcript_id: { type: "string" },
      quote_id: { type: "string" },
      client_name: { type: "string" },
      customer_name: { type: "string" },
      company: { type: "string" },
      currency: { type: "string" },
      discount_percent: { type: "number" },
      discount_cents: { type: "integer" },
      subtotal_cents: { type: "integer" },
      total_cents: { type: "integer" },
      grand_total: { type: "number" },
      valid_until: { type: "string" },
      line_items: { type: "array" },
      items: { type: "array" },
      crm_sync_payload: { type: "object" },
      crm_payload: { type: "object" },
      approval_id: { type: "string" },
    },
  },
  weekly_report: {
    type: "object",
    properties: {
      report_title: { type: "string" },
      date_range: { type: "string" },
      week_ending: { type: "string" },
      departments: { type: "array" },
      summary_kpis: { type: "object" },
      kpis: { type: "object" },
      markdown_report: { type: "string" },
      sections: { type: "array" },
    },
  },
  operations_weekly_report: {
    type: "object",
    properties: {
      report_title: { type: "string" },
      date_range: { type: "string" },
      week_ending: { type: "string" },
      departments: { type: "array" },
      summary_kpis: { type: "object" },
      kpis: { type: "object" },
      markdown_report: { type: "string" },
      sections: { type: "array" },
    },
  },
  script_storyboard: {
    type: "object",
    properties: {
      brief_id: { type: "string" },
      campaign_title: { type: "string" },
      project_name: { type: "string" },
      target_seconds: { type: "integer" },
      target_duration_seconds: { type: "integer" },
      aspect_ratio: { type: "string" },
      scenes: { type: "array" },
    },
  },
  marketing_storyboard: {
    type: "object",
    properties: {
      brief_id: { type: "string" },
      campaign_title: { type: "string" },
      project_name: { type: "string" },
      target_seconds: { type: "integer" },
      target_duration_seconds: { type: "integer" },
      aspect_ratio: { type: "string" },
      scenes: { type: "array" },
    },
  },
  email_manage: {
    type: "object",
    properties: {
      total_unread: { type: "integer" },
      total_scanned: { type: "integer" },
      unread_only: { type: "boolean" },
      limit: { type: "integer" },
      emails: { type: "array" },
      threads: { type: "array" },
      approval_id: { type: "string" },
    },
  },
  mail_draft_create: {
    type: "object",
    properties: {
      to: { type: "string" },
      recipient: { type: "string" },
      subject: { type: "string" },
      body: { type: "string" },
      cc: { type: "array" },
      approval_id: { type: "string" },
      approvalId: { type: "string" },
    },
  },
  create_email_draft: {
    type: "object",
    properties: {
      to: { type: "string" },
      recipient: { type: "string" },
      subject: { type: "string" },
      body: { type: "string" },
      cc: { type: "array" },
      approval_id: { type: "string" },
      approvalId: { type: "string" },
    },
  },
  mail_send_reply: {
    type: "object",
    properties: {
      to: { type: "string" },
      recipient: { type: "string" },
      subject: { type: "string" },
      body: { type: "string" },
      cc: { type: "array" },
      approval_id: { type: "string" },
      approvalId: { type: "string" },
    },
  },
  send_email: {
    type: "object",
    properties: {
      to: { type: "string" },
      recipient: { type: "string" },
      subject: { type: "string" },
      body: { type: "string" },
      cc: { type: "array" },
      approval_id: { type: "string" },
      approvalId: { type: "string" },
    },
  },
  support_escalate: {
    type: "object",
    properties: {
      ticket_id: { type: "string" },
      customer_email: { type: "string" },
      plan_tier: { type: "string" },
      severity: { type: "string" },
      sentiment: { type: "string" },
      sentiment_score: { type: "number" },
      high_risk: { type: "boolean" },
      issue_summary: { type: "string" },
      resolution_text: { type: "string" },
      kb_citations: { type: "array" },
      kb_matches: { type: "array" },
      escalation: { type: "object" },
      approval_id: { type: "string" },
    },
  },
  financial_analysis: {
    type: "object",
    properties: {
      fiscal_quarter: { type: "string" },
      period: { type: "string" },
      budget_cents: { type: "integer" },
      planned_total: { type: "number" },
      actual_cents: { type: "integer" },
      actual_total: { type: "number" },
      variance_cents: { type: "integer" },
      variance_total: { type: "number" },
      variance_pct: { type: "number" },
      status: { type: "string" },
      slides: { type: "array" },
    },
  },
};

/** Register the domain cards and standard schemas. Idempotent. */
export function registerDomainCards(): void {
  const registrations: Array<[string, ToolCardComponent]> = [
    ["links_create", LinkCard],
    ["links_update", LinkCard],
    ["links_get", LinkCard],
    ["links_list", LinkCard],
    ["links_disable", LinkCard],
    ["links_batch_disable", LinkCard],
    ["crew_create", EmployeeCard],
    ["crew_get", EmployeeCard],
    ["crew_list", EmployeeCard],
    ["crew_update_soul", EmployeeCard],
    ["orders_get", OrderCard],
    ["orders_list", OrderCard],
    ["finance_balance", FinanceCard],
    ["finance_list_entries", FinanceCard],
    ["finance_list_subscriptions", FinanceCard],
    ["statuspage_public", StatusCard],
    ["statuspage_probe", StatusCard],
    ["portal_licenses", PortalCard],
    ["portal_devices", PortalCard],

    // 8 Enterprise Knowledge Work Cards
    ["recruiting_screen", ResumeRankCard],
    ["invoice_audit", InvoiceAuditCard],
    ["quotation_generate", QuotationCard],
    ["sales_quotation", QuotationCard],
    ["weekly_report", WeeklyReportCard],
    ["operations_weekly_report", WeeklyReportCard],
    ["script_storyboard", ScriptStoryboardCard],
    ["marketing_storyboard", ScriptStoryboardCard],
    ["email_manage", EmailManagementCard],
    ["mail_draft_create", EmailManagementCard],
    ["create_email_draft", EmailManagementCard],
    ["mail_send_reply", EmailManagementCard],
    ["send_email", EmailManagementCard],
    ["support_escalate", SupportEscalationCard],
    ["financial_analysis", FinancialAnalysisCard],
  ];

  for (const [name, component] of registrations) {
    if (!resolveCard(name)) {
      registerCard(name, component);
    }
  }

  for (const [name, schema] of Object.entries(DEFAULT_SCHEMAS)) {
    if (!schemasFor(name)) {
      registerSchema(name, schema);
    }
  }
}

// Initial bootstrap
registerDomainCards();
