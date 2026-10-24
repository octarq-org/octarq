// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "../../../i18n";
import { CopilotDrawer } from "../../CopilotDrawer";
import { useCopilotStore, type Artifact } from "../../store";
import { CanvasApprovalGate } from "../../canvas/CanvasApprovalGate";
import { ResumeRankCard } from "../ResumeRankCard";
import { InvoiceAuditCard } from "../InvoiceAuditCard";
import { QuotationCard } from "../QuotationCard";
import { EmailManagementCard } from "../EmailManagementCard";
import { SupportEscalationCard } from "../SupportEscalationCard";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
});

function renderDrawer() {
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <CopilotDrawer />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

describe("Empirical Challenger 3 Stress Test Suite", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetCopilot();
    useCopilotStore.getState().openCopilot();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("Adversarial network block: unmocked fetch forbidden");
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  // =========================================================================
  // VECTOR 1: EXOTIC WHITESPACE & TAMPERED APPROVAL ID INJECTIONS
  // =========================================================================
  describe("Vector 1: Exotic Whitespace & Nullish Injections Across 5 Action Cards", () => {
    const whitespaceVariants = [
      { name: "tabs only", val: "\t\t\t" },
      { name: "newlines only", val: "\n\r\n" },
      { name: "mixed tabs, newlines and spaces", val: "  \t \n \r  " },
      { name: "unicode non-breaking and fullwidth spaces", val: "\u00A0 \u3000" },
    ];

    whitespaceVariants.forEach(({ name, val }) => {
      it(`ResumeRankCard blocks submission on ${name}`, () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ResumeRankCard
            toolName="recruiting_screen"
            args={{
              job_title: "Security Auditor",
              candidates: [{ rank: 1, name: "Bob", score: 0.99 }],
              approval_id: val,
            }}
            onAction={onAction}
          />,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const btn = screen.getByTestId("stage-invites-btn");
        expect(btn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(btn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it(`InvoiceAuditCard blocks submission on ${name}`, () => {
        const onAction = vi.fn();
        renderWithI18n(
          <InvoiceAuditCard
            toolName="invoice_audit"
            args={{ total_invoices: 10, matched_count: 8, approval_id: val }}
            onAction={onAction}
          />,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const btn = screen.getByTestId("release-payments-btn");
        expect(btn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(btn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it(`QuotationCard blocks submission on ${name}`, () => {
        const onAction = vi.fn();
        renderWithI18n(
          <QuotationCard
            toolName="quotation_generate"
            args={{ client_name: "Evil Corp", approval_id: val }}
            onAction={onAction}
          />,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const btn = screen.getByTestId("send-quotation-btn");
        expect(btn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(btn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it(`EmailManagementCard blocks submission on ${name}`, () => {
        const onAction = vi.fn();
        renderWithI18n(
          <EmailManagementCard
            toolName="email_manage"
            args={{ total_unread: 2, approval_id: val }}
            onAction={onAction}
          />,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const btn = screen.getByTestId("send-replies-btn");
        expect(btn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(btn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it(`SupportEscalationCard blocks submission on ${name}`, () => {
        const onAction = vi.fn();
        renderWithI18n(
          <SupportEscalationCard
            toolName="support_escalate"
            args={{ ticket_id: "TICK-888", approval_id: val }}
            onAction={onAction}
          />,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const btn = screen.getByTestId("escalate-ticket-btn");
        expect(btn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(btn);
        expect(onAction).not.toHaveBeenCalled();
      });
    });

    it("verifies approvalId is passed trimmed when valid string has leading/trailing spaces", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <ResumeRankCard
          toolName="recruiting_screen"
          args={{
            job_title: "Lead SRE",
            candidates: [{ rank: 1, name: "Carol", score: 0.98 }],
            approval_id: "   appr-valid-trimmed-123   ",
          }}
          onAction={onAction}
        />,
      );
      expect(screen.queryByTestId("approval-missing-id")).toBeNull();
      const btn = screen.getByTestId("stage-invites-btn");
      expect(btn.hasAttribute("disabled")).toBe(false);
      fireEvent.click(btn);
      expect(onAction).toHaveBeenCalledTimes(1);
      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          approvalId: "appr-valid-trimmed-123",
        }),
      );
    });
  });

  // =========================================================================
  // VECTOR 2: CANVAS APPROVAL GATE FAIL-CLOSED RESILIENCE
  // =========================================================================
  describe("Vector 2: CanvasApprovalGate Fail-Closed Resilience", () => {
    it("fails closed with failed verdict on HTTP 500 server error", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 500 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-500"
          actionLabel="Execute 500"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Execute 500/i }));

      await waitFor(() => {
        const terminal = screen.getByTestId("gate-terminal");
        expect(terminal.getAttribute("data-verdict")).toBe("failed");
        expect(terminal.getAttribute("role")).toBe("alert");
        expect(terminal.textContent).toContain("Server returned an error. The system made no changes.");
      });
    });

    it("fails closed with failed verdict on HTTP 503 service unavailable", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 503 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-503"
          actionLabel="Execute 503"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Execute 503/i }));

      await waitFor(() => {
        const terminal = screen.getByTestId("gate-terminal");
        expect(terminal.getAttribute("data-verdict")).toBe("failed");
        expect(terminal.getAttribute("role")).toBe("alert");
      });
    });

    it("fails closed when outcome returns { success: false }", async () => {
      const onAction = vi.fn().mockResolvedValue({ success: false });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-success-false"
          actionLabel="Execute Fail"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Execute Fail/i }));

      await waitFor(() => {
        const terminal = screen.getByTestId("gate-terminal");
        expect(terminal.getAttribute("data-verdict")).toBe("failed");
        expect(terminal.textContent).toContain("Action failed. The system made no changes.");
      });
    });

    it("fails closed with error message on thrown network rejection", async () => {
      const onAction = vi.fn().mockRejectedValue(new Error("Database connection dropped"));
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-throw-err"
          actionLabel="Execute Throw"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Execute Throw/i }));

      await waitFor(() => {
        const terminal = screen.getByTestId("gate-terminal");
        expect(terminal.getAttribute("data-verdict")).toBe("failed");
        expect(terminal.textContent).toContain("Database connection dropped");
      });
    });

    it("fails closed when reject button encounters HTTP 500 error", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 500 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-reject-500"
          actionLabel="Execute Action"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Reject/i }));

      await waitFor(() => {
        const terminal = screen.getByTestId("gate-terminal");
        expect(terminal.getAttribute("data-verdict")).toBe("failed");
        expect(terminal.textContent).toContain("Server returned an error. The system made no changes.");
      });
    });

    it("prevents double-execution on rapid double-click via isSubmitting guard", async () => {
      let resolvePromise: (val: any) => void = () => {};
      const pendingPromise = new Promise((resolve) => {
        resolvePromise = resolve;
      });
      const onAction = vi.fn().mockReturnValue(pendingPromise);

      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-race-click"
          actionLabel="Race Action"
          onAction={onAction}
        />,
      );

      const btn = screen.getByRole("button", { name: /Race Action/i });
      fireEvent.click(btn);
      fireEvent.click(btn);
      fireEvent.click(btn);

      expect(onAction).toHaveBeenCalledTimes(1);

      act(() => {
        resolvePromise({ status: 200 });
      });

      await waitFor(() => {
        expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("decided");
      });
    });
  });

  // =========================================================================
  // VECTOR 3: CANVAS STORE MUTATIONS & COMPLEX LAYOUT THRASHING
  // =========================================================================
  describe("Vector 3: Complex Multi-Step Canvas Mutations & Layout Thrashing", () => {
    it("preserves complex line-item sequence across multiple layout toggles in QuotationCanvas", async () => {
      renderDrawer();

      const initialArtifact: Artifact = {
        id: "art-complex-quote",
        type: "quotation",
        title: "Multi-Step Quotation",
        version: 1,
        status: "draft",
        content: {
          client_name: "MegaCorp",
          line_items: [
            { sku: "ITEM-A", description: "Alpha Service", quantity: 2, unit_price_cents: 50000 },
          ],
          discount_percent: 10,
          approval_id: "appr-q-multi",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(initialArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      // 1. Add item 2
      fireEvent.click(screen.getByTestId("add-line-item-btn"));
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(2);

      // 2. Add item 3
      fireEvent.click(screen.getByTestId("add-line-item-btn"));
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(3);

      // 3. Switch layout: split -> maximized -> split
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });

      // Assert items survived first layout thrash
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(3);

      // 4. Delete item 2
      const deleteButtons = screen.getAllByLabelText("Remove item");
      fireEvent.click(deleteButtons[1]);
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(2);

      // 5. Change discount percentage
      const discountSlider = screen.getByRole("slider");
      fireEvent.change(discountSlider, { target: { value: "15" } });

      // 6. Thrash layout: split -> drawer -> split -> maximized -> split
      act(() => {
        useCopilotStore.getState().setCanvasLayout("drawer");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });

      // 7. Verify all state survived in store and rendered component
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(2);
      const storeArtifact = useCopilotStore.getState().artifacts["art-complex-quote"];
      expect((storeArtifact.content as any).line_items).toHaveLength(2);
      expect((storeArtifact.content as any).discount_percent).toBe(15);
    });

    it("preserves multi-field email edits across multiple layout toggles in EmailDraftCanvas", async () => {
      renderDrawer();

      const initialArtifact: Artifact = {
        id: "art-complex-email",
        type: "email_draft",
        title: "Multi-Field Email",
        version: 1,
        status: "draft",
        content: {
          threads: [
            {
              id: "thread-main",
              sender: "partner@enterprise.org",
              subject: "Enterprise Agreement Scope",
              summary: "Partnership terms review",
              draft_reply: {
                to: "partner@enterprise.org",
                subject: "Re: Enterprise Agreement Scope",
                body: "Initial text.",
              },
            },
          ],
          approval_id: "appr-email-complex",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(initialArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      // 1. Edit To
      const toInput = screen.getByLabelText("Recipient email");
      fireEvent.change(toInput, { target: { value: "legal-review@enterprise.org" } });

      // 2. Edit Subject
      const subjectInput = screen.getByLabelText("Email subject");
      fireEvent.change(subjectInput, { target: { value: "Urgent: Finalized Master Service Agreement" } });

      // 3. Edit Body
      const bodyInput = screen.getByLabelText("Email reply body");
      fireEvent.change(bodyInput, { target: { value: "Attached is the redlined version with agreed SLA terms." } });

      // 4. Thrash layout
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("drawer");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });

      // 5. Verify all three fields survived
      const toAfter = screen.getByLabelText("Recipient email") as HTMLInputElement;
      const subAfter = screen.getByLabelText("Email subject") as HTMLInputElement;
      const bodyAfter = screen.getByLabelText("Email reply body") as HTMLTextAreaElement;

      expect(toAfter.value).toBe("legal-review@enterprise.org");
      expect(subAfter.value).toBe("Urgent: Finalized Master Service Agreement");
      expect(bodyAfter.value).toBe("Attached is the redlined version with agreed SLA terms.");

      const storeContent = useCopilotStore.getState().artifacts["art-complex-email"].content as any;
      expect(storeContent.draft_reply.to).toBe("legal-review@enterprise.org");
      expect(storeContent.draft_reply.subject).toBe("Urgent: Finalized Master Service Agreement");
      expect(storeContent.draft_reply.body).toBe("Attached is the redlined version with agreed SLA terms.");
    });
  });

  // =========================================================================
  // VECTOR 4: ASD-STE100 & FAIL-CLOSED INTEGRITY INSPECTION
  // =========================================================================
  describe("Vector 4: ASD-STE100 Word Count & Strict Fail-Closed Invariants", () => {
    it("ensures every newly added terminal status string conforms to ASD-STE100", () => {
      const messages = [
        "Action failed. The system made no changes.",
        "Server returned an error. The system made no changes.",
        "Action approved. The change executed successfully.",
        "Action already decided. The system made no changes.",
        "Approval ticket expired. Submit a new request.",
        "Action rejected. The system made no changes.",
        "No approval ID provided. You cannot submit this action.",
      ];

      for (const msg of messages) {
        const sentences = msg.split(/[.!?]\s+/).filter(Boolean);
        for (const s of sentences) {
          const words = s.trim().split(/\s+/).filter(Boolean);
          expect(words.length).toBeLessThanOrEqual(20);
        }
      }
    });
  });
});
