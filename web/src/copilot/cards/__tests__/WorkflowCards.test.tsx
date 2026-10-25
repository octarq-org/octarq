// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { I18nProvider } from "../../../i18n";
import { useCopilotStore } from "../../store";

import { ResumeRankCard } from "../ResumeRankCard";
import { InvoiceAuditCard } from "../InvoiceAuditCard";
import { QuotationCard } from "../QuotationCard";
import { WeeklyReportCard } from "../WeeklyReportCard";
import { ScriptStoryboardCard } from "../ScriptStoryboardCard";
import { EmailManagementCard } from "../EmailManagementCard";
import { SupportEscalationCard } from "../SupportEscalationCard";
import { FinancialAnalysisCard } from "../FinancialAnalysisCard";

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

const noop = vi.fn();

function assertSTE100(container: HTMLElement) {
  const text = container.textContent || "";
  // No robotic filler
  expect(text).not.toMatch(/\b(please|simply|just|delve|seamlessly|obviously)\b/i);
}

describe("8 Workflow Visual Action Cards — 4 States & ASD-STE100", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetCopilot();
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  describe("1. ResumeRankCard (recruiting_screen)", () => {
    it("renders loading state", () => {
      const { container } = renderWithI18n(
        <ResumeRankCard toolName="recruiting_screen" args={{}} state="loading" onAction={noop} />,
      );
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      expect(screen.getByText("Loading data…")).toBeDefined();
      assertSTE100(container);
    });

    it("renders error state with detail", () => {
      const { container } = renderWithI18n(
        <ResumeRankCard
          toolName="recruiting_screen"
          args={{ errorDetail: "Failed to parse candidate resume PDF" }}
          state="error"
          onAction={noop}
        />,
      );
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      expect(screen.getByText(/Failed to parse candidate resume PDF/)).toBeDefined();
      assertSTE100(container);
    });

    it("renders empty state", () => {
      const { container } = renderWithI18n(
        <ResumeRankCard toolName="recruiting_screen" args={{}} state="empty" onAction={noop} />,
      );
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      expect(screen.getByText("No items to display.")).toBeDefined();
      assertSTE100(container);
    });

    it("renders forbidden entitlement state", () => {
      const { container } = renderWithI18n(
        <ResumeRankCard
          toolName="recruiting_screen"
          args={{ forbiddenReason: "Enterprise HR screening license required." }}
          state="forbidden"
          onAction={noop}
        />,
      );
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      expect(screen.getByText("Enterprise HR screening license required.")).toBeDefined();
      assertSTE100(container);
    });

    it("renders ok state with candidates and dispatches open_canvas action", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <ResumeRankCard
          toolName="recruiting_screen"
          args={{
            job_title: "Senior Distributed Systems Engineer",
            total_candidates: 40,
            shortlist_count: 5,
            candidates: [
              { rank: 1, name: "Alice Chen", score: 0.98, current_title: "Staff Engineer" },
              { rank: 2, name: "Marcus Vance", score: 0.94, current_title: "Senior SRE" },
              { rank: 3, name: "Sophia Patel", score: 0.91, current_title: "Principal Engineer" },
            ],
            approval_id: "appr-recruiting-001",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("job-title").textContent).toContain("Senior Distributed Systems Engineer");
      expect(screen.getByText("Alice Chen")).toBeDefined();
      expect(screen.getByText("98%")).toBeDefined();
      assertSTE100(container);

      // Open in Canvas
      fireEvent.click(screen.getByTestId("open-canvas-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "open_canvas",
          toolName: "recruiting_screen",
        }),
      );
      expect(useCopilotStore.getState().activeArtifactId).toBeDefined();

      // Stage Invites (with approvalId)
      fireEvent.click(screen.getByTestId("stage-invites-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "approve",
          approvalId: "appr-recruiting-001",
        }),
      );
    });
  });

  describe("2. InvoiceAuditCard (invoice_audit)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<InvoiceAuditCard toolName="invoice_audit" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<InvoiceAuditCard toolName="invoice_audit" args={{ errorDetail: "Database timeout" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<InvoiceAuditCard toolName="invoice_audit" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<InvoiceAuditCard toolName="invoice_audit" args={{ forbiddenReason: "Pro license required." }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with metrics and enforces approval gate when approvalId is missing", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <InvoiceAuditCard
          toolName="invoice_audit"
          args={{
            total_invoices: 54,
            matched_count: 36,
            discrepancy_count: 18,
            overdue_count: 6,
            audit_time_seconds: 42,
            discrepancies: [
              { invoice_no: "INV-104", vendor: "Acme Logistics", delta_cents: 175000, issue: "Overbilled" },
            ],
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("metric-scanned").textContent).toBe("54");
      expect(screen.getByTestId("metric-matched").textContent).toBe("36");
      expect(screen.getByTestId("metric-discrepancies").textContent).toBe("18");
      expect(screen.getByTestId("approval-missing-id")).toBeDefined();
      expect(screen.getByTestId("release-payments-btn").hasAttribute("disabled")).toBe(true);
      assertSTE100(container);

      // Open in Canvas
      fireEvent.click(screen.getByTestId("open-canvas-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "open_canvas", toolName: "invoice_audit" }),
      );
    });
  });

  describe("3. QuotationCard (quotation_generate / sales_quotation)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<QuotationCard toolName="quotation_generate" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<QuotationCard toolName="quotation_generate" args={{ errorDetail: "Transcript parse error" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<QuotationCard toolName="quotation_generate" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<QuotationCard toolName="quotation_generate" args={{ forbiddenReason: "Sales tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with client quotation details and handles send action", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <QuotationCard
          toolName="quotation_generate"
          args={{
            quote_id: "QUOTE-2026-901",
            client_name: "Acme Cloud Logistics",
            company: "Acme Logistics Inc.",
            total_cents: 2500000,
            line_items: [
              { sku: "OCT-ELITE", description: "Octarq Pro Elite License", total_cents: 2000000 },
              { sku: "OCT-REDIRECTS", description: "High-Volume Redirect Pack", total_cents: 500000 },
            ],
            approval_id: "appr-quote-001",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("client-name").textContent).toContain("Acme Cloud Logistics");
      expect(screen.getByTestId("total-amount").textContent).toContain("$25,000.00");
      expect(screen.getByTestId("send-quotation-btn").hasAttribute("disabled")).toBe(false);
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("send-quotation-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "approve",
          approvalId: "appr-quote-001",
        }),
      );
    });
  });

  describe("4. WeeklyReportCard (weekly_report / operations_weekly_report)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<WeeklyReportCard toolName="weekly_report" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<WeeklyReportCard toolName="weekly_report" args={{ errorDetail: "Aggregation failed" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<WeeklyReportCard toolName="weekly_report" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<WeeklyReportCard toolName="weekly_report" args={{ forbiddenReason: "Reporting tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with KPIs and handles open canvas", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <WeeklyReportCard
          toolName="weekly_report"
          args={{
            report_title: "Executive Operations Weekly Report",
            date_range: "Oct 18 – Oct 24, 2026",
            summary_kpis: {
              traffic_delta: "+18.4%",
              mail_delivery_rate: "99.8%",
              invoice_matched_pct: 0.94,
              open_tickets: 0,
            },
            markdown_report: "# Weekly Summary\nAll services operational.",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("report-title").textContent).toContain("Executive Operations Weekly Report");
      expect(screen.getByText("+18.4%")).toBeDefined();
      expect(screen.getByText("99.8%")).toBeDefined();
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("open-canvas-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "open_canvas", toolName: "weekly_report" }),
      );
    });
  });

  describe("5. ScriptStoryboardCard (script_storyboard / marketing_storyboard)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<ScriptStoryboardCard toolName="script_storyboard" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<ScriptStoryboardCard toolName="script_storyboard" args={{ errorDetail: "Brief parse error" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<ScriptStoryboardCard toolName="script_storyboard" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<ScriptStoryboardCard toolName="script_storyboard" args={{ forbiddenReason: "Marketing tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with scenes and dispatches export action", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <ScriptStoryboardCard
          toolName="script_storyboard"
          args={{
            campaign_title: "AI Executive Assistant Launch",
            target_seconds: 60,
            aspect_ratio: "16:9",
            scenes: [
              { scene_number: 1, duration_sec: 12, visual_prompt: "Engineering leader looking at busy dashboard", shot_type: "Wide Aerial" },
            ],
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("campaign-title").textContent).toContain("AI Executive Assistant Launch");
      expect(screen.getByTestId("scene-preview")).toBeDefined();
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("export-shots-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "custom", toolName: "export_shot_list" }),
      );
    });
  });

  describe("6. EmailManagementCard (email_manage)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<EmailManagementCard toolName="email_manage" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<EmailManagementCard toolName="email_manage" args={{ errorDetail: "IMAP connection dropped" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<EmailManagementCard toolName="email_manage" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<EmailManagementCard toolName="email_manage" args={{ forbiddenReason: "Mail tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with email count and staged drafts", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <EmailManagementCard
          toolName="email_manage"
          args={{
            total_unread: 5,
            threads: [
              { id: "mail-1", sender: "client@acme.com", subject: "Contract Renewal Question", summary: "Inquires about adding 50 seats", urgency: "urgent" },
            ],
            approval_id: "appr-mail-001",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("email-count").textContent).toContain("5 Inbound Messages");
      expect(screen.getByText("Contract Renewal Question")).toBeDefined();
      expect(screen.getByTestId("send-replies-btn").hasAttribute("disabled")).toBe(false);
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("send-replies-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "approve", approvalId: "appr-mail-001" }),
      );
    });
  });

  describe("7. SupportEscalationCard (support_escalate)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<SupportEscalationCard toolName="support_escalate" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<SupportEscalationCard toolName="support_escalate" args={{ errorDetail: "Ticket system unreachable" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<SupportEscalationCard toolName="support_escalate" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<SupportEscalationCard toolName="support_escalate" args={{ forbiddenReason: "Support tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with critical incident escalation and owner routing", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <SupportEscalationCard
          toolName="support_escalate"
          args={{
            ticket_id: "TICK-9081",
            customer_email: "vp@enterprise.com",
            severity: "CRITICAL",
            high_risk: true,
            escalation: {
              recommended_owner: "VP of Infrastructure",
              root_cause_summary: "Production DNS redirect failure loop.",
            },
            approval_id: "appr-escalate-001",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("ticket-id").textContent).toBe("TICK-9081");
      expect(screen.getByTestId("severity-chip").textContent).toBe("CRITICAL");
      expect(screen.getByText("Production DNS redirect failure loop.")).toBeDefined();
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("escalate-ticket-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "approve", approvalId: "appr-escalate-001" }),
      );
    });
  });

  describe("8. FinancialAnalysisCard (financial_analysis)", () => {
    it("renders loading, error, empty, forbidden states", () => {
      renderWithI18n(<FinancialAnalysisCard toolName="financial_analysis" args={{}} state="loading" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
      cleanup();

      renderWithI18n(<FinancialAnalysisCard toolName="financial_analysis" args={{ errorDetail: "Ledger read error" }} state="error" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
      cleanup();

      renderWithI18n(<FinancialAnalysisCard toolName="financial_analysis" args={{}} state="empty" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
      cleanup();

      renderWithI18n(<FinancialAnalysisCard toolName="financial_analysis" args={{ forbiddenReason: "Finance tier required" }} state="forbidden" onAction={noop} />);
      expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
      cleanup();
    });

    it("renders ok state with variance metrics and slides preview", () => {
      const onAction = vi.fn();
      const { container } = renderWithI18n(
        <FinancialAnalysisCard
          toolName="financial_analysis"
          args={{
            fiscal_quarter: "FY26 Q3",
            budget_cents: 34500000,
            actual_cents: 41060000,
            variance_cents: 6560000,
            variance_pct: 19.0,
            slides: [
              { slide_number: 1, title: "Executive Variance Snapshot", bullets: ["Budget overrun in cloud infra"] },
              { slide_number: 2, title: "Root Cause Breakdown", bullets: ["GPU development clusters"] },
              { slide_number: 3, title: "Corrective Actions", bullets: ["Automated TTL shutdown"] },
            ],
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("fiscal-quarter").textContent).toContain("FY26 Q3");
      expect(screen.getByTestId("variance-badge").textContent).toContain("+19.0% Over");
      expect(screen.getByTestId("slides-preview")).toBeDefined();
      assertSTE100(container);

      fireEvent.click(screen.getByTestId("open-canvas-btn"));
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({ type: "open_canvas", toolName: "financial_analysis" }),
      );
    });
  });
});
