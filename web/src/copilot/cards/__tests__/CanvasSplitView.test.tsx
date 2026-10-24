// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "../../../i18n";
import { CopilotDrawer } from "../../CopilotDrawer";
import { useCopilotStore, type Artifact } from "../../store";
import { CanvasApprovalGate } from "../../canvas/CanvasApprovalGate";
import { QuotationCanvas } from "../../canvas/editors/QuotationCanvas";
import { ResumeRankCanvas } from "../../canvas/editors/ResumeRankCanvas";
import { WeeklyReportCanvas } from "../../canvas/editors/WeeklyReportCanvas";
import { EmailDraftCanvas } from "../../canvas/editors/EmailDraftCanvas";
import { FinancialDeckCanvas } from "../../canvas/editors/FinancialDeckCanvas";

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

describe("OpenHuman Canvas Split-View & Approval Gate", () => {
  beforeEach(() => {
    useCopilotStore.getState().resetCopilot();
    useCopilotStore.getState().openCopilot();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("network offline");
      }),
    );
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  describe("1. Layout Mode Switching (drawer -> split -> maximized -> drawer)", () => {
    it("starts in drawer mode when no artifact is open", () => {
      renderDrawer();
      const dialog = screen.getByRole("dialog");
      expect(dialog.getAttribute("data-layout")).toBe("drawer");
      expect(screen.queryByTestId("copilot-canvas-pane")).toBeNull();
    });

    it("switches to split view when openArtifact is called", async () => {
      renderDrawer();

      const sampleArtifact: Artifact = {
        id: "art-1",
        type: "weekly_report",
        title: "Operations Weekly Report",
        version: 1,
        status: "draft",
        content: {
          report_title: "Operations Weekly Report",
          markdown_report: "# Summary\nAll services operational.",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(sampleArtifact);
      });

      await waitFor(() => {
        const dialog = screen.getByRole("dialog");
        expect(dialog.getAttribute("data-layout")).toBe("split");
      });

      expect(screen.getByTestId("copilot-chat-pane")).toBeDefined();
      expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      expect(screen.getByTestId("artifact-title").textContent).toContain("Operations Weekly Report");
    });

    it("toggles to maximized layout and back to split", async () => {
      renderDrawer();

      const sampleArtifact: Artifact = {
        id: "art-max-1",
        type: "financial_deck",
        title: "Q3 Financial Review",
        version: 1,
        status: "draft",
        content: { fiscal_quarter: "FY26 Q3" },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(sampleArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("maximize-canvas-btn")).toBeDefined();
      });

      // Maximize
      const maxBtn = screen.getByTestId("maximize-canvas-btn");
      fireEvent.click(maxBtn);

      await waitFor(() => {
        expect(useCopilotStore.getState().canvasLayout).toBe("maximized");
        const dialog = screen.getByRole("dialog");
        expect(dialog.getAttribute("data-layout")).toBe("maximized");
      });

      // Restore Split
      const restoreBtn = screen.getByTestId("restore-split-btn");
      fireEvent.click(restoreBtn);

      await waitFor(() => {
        expect(useCopilotStore.getState().canvasLayout).toBe("split");
        const dialog = screen.getByRole("dialog");
        expect(dialog.getAttribute("data-layout")).toBe("split");
      });
    });

    it("closes canvas cleanly and returns to drawer mode", async () => {
      renderDrawer();

      const sampleArtifact: Artifact = {
        id: "art-close-1",
        type: "weekly_report",
        title: "Test Report",
        version: 1,
        status: "draft",
        content: {},
      };

      act(() => {
        useCopilotStore.getState().openArtifact(sampleArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      const closeBtn = screen.getByTestId("close-canvas-btn");
      fireEvent.click(closeBtn);

      await waitFor(() => {
        expect(useCopilotStore.getState().activeArtifactId).toBeNull();
        expect(useCopilotStore.getState().canvasLayout).toBe("drawer");
        expect(screen.queryByTestId("copilot-canvas-pane")).toBeNull();
      });
    });
  });

  describe("2. Interactive Canvas Editing", () => {
    it("updates line item quantities and discounts in QuotationCanvas", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <QuotationCanvas
          artifactId="art-quote-1"
          content={{
            client_name: "Acme Cloud",
            line_items: [
              { sku: "SKU-A", description: "Standard Enterprise", quantity: 1, unit_price_cents: 100000 },
            ],
            approval_id: "appr-quote-999",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("grand-total").textContent).toBe("$920.00"); // 1000 - 8% discount

      // Add Item
      const addBtn = screen.getByTestId("add-line-item-btn");
      fireEvent.click(addBtn);

      const rows = screen.getAllByTestId("line-item-row");
      expect(rows).toHaveLength(2);
    });

    it("filters and inspects candidates in ResumeRankCanvas", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <ResumeRankCanvas
          artifactId="art-resume-1"
          content={{
            job_title: "Lead Go Developer",
            candidates: [
              { rank: 1, name: "Alice Chen", score: 0.98, current_title: "Senior Architect", strengths: ["Raft", "Go"] },
              { rank: 2, name: "Bob Smith", score: 0.85, current_title: "Fullstack Eng", strengths: ["React"] },
            ],
            approval_id: "appr-res-001",
          }}
          onAction={onAction}
        />,
      );

      const rows = screen.getAllByTestId("candidate-row");
      expect(rows).toHaveLength(2);

      // Search
      const searchInput = screen.getByPlaceholderText("Filter by name or skill…");
      fireEvent.change(searchInput, { target: { value: "Alice" } });

      const filtered = screen.getAllByTestId("candidate-row");
      expect(filtered).toHaveLength(1);
      expect(screen.getByText("Alice Chen")).toBeDefined();

      // Inspect
      const inspectBtn = screen.getByRole("button", { name: "Inspect" });
      fireEvent.click(inspectBtn);
      expect(screen.getByTestId("candidate-detail-modal")).toBeDefined();
    });

    it("edits markdown source in WeeklyReportCanvas and updates preview", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <WeeklyReportCanvas
          artifactId="art-report-1"
          content={{
            report_title: "Weekly Update",
            markdown_report: "# Initial Title\nBody text here.",
          }}
          onAction={onAction}
        />,
      );

      const textarea = screen.getByLabelText("Markdown editor");
      fireEvent.change(textarea, { target: { value: "# Updated Executive Title\nNew operational metrics." } });

      expect(screen.getByTestId("markdown-preview").textContent).toContain("Updated Executive Title");
    });

    it("applies tone presets in EmailDraftCanvas", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <EmailDraftCanvas
          artifactId="art-email-1"
          content={{
            threads: [
              {
                id: "mail-1",
                sender: "partner@example.com",
                subject: "Inquiry",
                summary: "Summary text",
                draft_reply: { to: "partner@example.com", subject: "Re: Inquiry", body: "Original body" },
              },
            ],
            approval_id: "appr-email-1",
          }}
          onAction={onAction}
        />,
      );

      expect(screen.getByTestId("untrusted-email-container")).toBeDefined();
      const conciseBtn = screen.getByRole("button", { name: "Concise" });
      fireEvent.click(conciseBtn);

      const bodyTextarea = screen.getByLabelText("Email reply body") as HTMLTextAreaElement;
      expect(bodyTextarea.value).toContain("sub-20ms latency");
    });

    it("navigates slides and toggles preview in FinancialDeckCanvas", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <FinancialDeckCanvas
          artifactId="art-deck-1"
          content={{ fiscal_quarter: "FY26 Q3" }}
          onAction={onAction}
        />,
      );

      expect(screen.getByText("SLIDE #1")).toBeDefined();
      expect(screen.getByText("SLIDE #2")).toBeDefined();
      expect(screen.getByText("SLIDE #3")).toBeDefined();

      // Switch to Slide 2
      fireEvent.click(screen.getByText("SLIDE #2"));
      expect(screen.getByText("PAGE #2 OF 3")).toBeDefined();

      // Toggle Preview
      const previewBtn = screen.getByRole("button", { name: "Preview Deck" });
      fireEvent.click(previewBtn);
      expect(screen.getByRole("button", { name: "Edit Mode" })).toBeDefined();
    });
  });

  describe("3. Fail-Closed CAS Approval Gate (CanvasApprovalGate)", () => {
    it("disables approve and reject buttons when approval ID is missing", () => {
      const onAction = vi.fn();
      renderWithI18n(
        <CanvasApprovalGate
          approvalId=""
          actionLabel="Execute Action"
          onAction={onAction}
        />,
      );

      const approveBtn = screen.getByRole("button", { name: /Execute Action/i });
      const rejectBtn = screen.getByRole("button", { name: /Reject/i });
      expect(approveBtn.hasAttribute("disabled")).toBe(true);
      expect(rejectBtn.hasAttribute("disabled")).toBe(true);
      expect(screen.getByTestId("approval-missing-id").textContent).toContain("No approval ID provided.");
    });

    it("approves action and enters terminal decided state on 200", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 200 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-cas-101"
          actionLabel="Release Invoices"
          onAction={onAction}
        />,
      );

      const approveBtn = screen.getByRole("button", { name: /Release Invoices/i });
      expect(approveBtn.hasAttribute("disabled")).toBe(false);
      fireEvent.click(approveBtn);

      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "approve",
          approvalId: "appr-cas-101",
        }),
      );

      await waitFor(() => {
        expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("decided");
        expect(screen.getByText("Action approved. The change executed successfully.")).toBeDefined();
      });
    });

    it("handles 409 conflict as terminal already decided", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 409 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-cas-102"
          actionLabel="Confirm Dispatch"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Confirm Dispatch/i }));

      await waitFor(() => {
        expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("decided");
        expect(screen.getByText("Action already decided. The system made no changes.")).toBeDefined();
      });
    });

    it("handles 410 gone as terminal expired state", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 410 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-cas-103"
          actionLabel="Confirm Dispatch"
          onAction={onAction}
        />,
      );

      fireEvent.click(screen.getByRole("button", { name: /Confirm Dispatch/i }));

      await waitFor(() => {
        expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("expired");
        expect(screen.getByText("Approval ticket expired. Submit a new request.")).toBeDefined();
      });
    });

    it("handles reject action", async () => {
      const onAction = vi.fn().mockResolvedValue({ status: 200 });
      renderWithI18n(
        <CanvasApprovalGate
          approvalId="appr-cas-104"
          actionLabel="Confirm Dispatch"
          onAction={onAction}
        />,
      );

      const rejectBtn = screen.getByRole("button", { name: /Reject/i });
      fireEvent.click(rejectBtn);

      expect(onAction).toHaveBeenCalledWith(
        expect.objectContaining({
          type: "reject",
          approvalId: "appr-cas-104",
        }),
      );

      await waitFor(() => {
        expect(screen.getByTestId("gate-terminal").getAttribute("data-verdict")).toBe("rejected");
        expect(screen.getByText("Action rejected. The system made no changes.")).toBeDefined();
      });
    });
  });
});
