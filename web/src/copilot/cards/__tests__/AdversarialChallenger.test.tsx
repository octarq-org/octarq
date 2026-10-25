// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { readdirSync, readFileSync, statSync } from "fs";
import { resolve, join } from "path";
import { I18nProvider } from "../../../i18n";
import { CopilotDrawer } from "../../CopilotDrawer";
import { useCopilotStore, type Artifact } from "../../store";
import { ApprovalCard } from "../ApprovalCard";
import { CanvasApprovalGate } from "../../canvas/CanvasApprovalGate";
import { ResumeRankCard } from "../ResumeRankCard";
import { InvoiceAuditCard } from "../InvoiceAuditCard";
import { QuotationCard } from "../QuotationCard";
import { EmailManagementCard } from "../EmailManagementCard";
import { SupportEscalationCard } from "../SupportEscalationCard";
import { WeeklyReportCanvas } from "../../canvas/editors/WeeklyReportCanvas";
import { QuotationCanvas } from "../../canvas/editors/QuotationCanvas";
import { ResumeRankCanvas } from "../../canvas/editors/ResumeRankCanvas";
import { EmailDraftCanvas } from "../../canvas/editors/EmailDraftCanvas";
import { SupportEscalationCanvas } from "../../canvas/editors/SupportEscalationCanvas";
import { StoryboardCanvas } from "../../canvas/editors/StoryboardCanvas";
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

describe("Adversarial Challenger Suite — Copilot Canvas & Action Cards", () => {
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
  // SECTION 1: APPROVAL GATE ADVERSARIAL ATTACKS
  // =========================================================================
  describe("1. Approval Gate Adversarial Attacks", () => {
    describe("ApprovalCard standalone attacks", () => {
      it("blocks submission and displays warning when approvalId is empty string", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: "" }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const approveBtn = screen.getByRole("button", { name: /Approve/i });
        const rejectBtn = screen.getByRole("button", { name: /Reject/i });
        expect(approveBtn.hasAttribute("disabled")).toBe(true);
        expect(rejectBtn.hasAttribute("disabled")).toBe(true);

        fireEvent.click(approveBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("blocks submission and displays warning when approvalId is whitespace only", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: "    " }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const approveBtn = screen.getByRole("button", { name: /Approve/i });
        expect(approveBtn.hasAttribute("disabled")).toBe(true);

        fireEvent.click(approveBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("blocks submission when approvalId is null or undefined", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: undefined, approvalId: undefined }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        expect(screen.getByRole("button", { name: /Approve/i }).hasAttribute("disabled")).toBe(true);
      });

      it("handles HTTP 409 already-resolved cleanly and transitions to terminal badge", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 409 });
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: "appr-valid-409" }}
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Approve/i });
        fireEvent.click(approveBtn);

        await waitFor(() => {
          const terminal = screen.getByTestId("approval-terminal");
          expect(terminal.getAttribute("data-verdict")).toBe("decided");
          expect(screen.queryByRole("button", { name: /Approve/i })).toBeNull();
          expect(screen.queryByRole("button", { name: /Reject/i })).toBeNull();
        });
      });

      it("handles HTTP 410 expired cleanly and transitions to terminal expired badge", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 410 });
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: "appr-valid-410" }}
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Approve/i });
        fireEvent.click(approveBtn);

        await waitFor(() => {
          const terminal = screen.getByTestId("approval-terminal");
          expect(terminal.getAttribute("data-verdict")).toBe("expired");
          expect(terminal.textContent).toContain("Approval expired. Submit a new request.");
          expect(screen.queryByRole("button", { name: /Approve/i })).toBeNull();
        });
      });

      it("fails closed on fabricated approvalId that returns HTTP 400 / 404 / 500", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 400 });
        renderWithI18n(
          <ApprovalCard
            toolName="mail_send_reply"
            args={{ approval_id: "fabricated-tampered-id-999" }}
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Approve/i });
        fireEvent.click(approveBtn);

        await waitFor(() => {
          expect(screen.getByRole("alert").textContent).toContain("Server returned status 400");
          expect(screen.queryByTestId("approval-terminal")).toBeNull();
        });
      });
    });

    describe("CanvasApprovalGate attacks", () => {
      it("blocks submission and shows approval-missing-id when approvalId is empty or whitespace", () => {
        const onAction = vi.fn();
        const { rerender } = renderWithI18n(
          <CanvasApprovalGate
            approvalId=""
            actionLabel="Dispatch Payment"
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const approveBtn = screen.getByRole("button", { name: /Dispatch Payment/i });
        expect(approveBtn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(approveBtn);
        expect(onAction).not.toHaveBeenCalled();

        // Whitespace test
        rerender(
          <I18nProvider>
            <CanvasApprovalGate
              approvalId="    "
              actionLabel="Dispatch Payment"
              onAction={onAction}
            />
          </I18nProvider>,
        );
        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        expect(screen.getByRole("button", { name: /Dispatch Payment/i }).hasAttribute("disabled")).toBe(true);
        fireEvent.click(screen.getByRole("button", { name: /Dispatch Payment/i }));
        expect(onAction).not.toHaveBeenCalled();
      });

      it("transitions to terminal badge on 409 conflict and halts any further action", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 409 });
        renderWithI18n(
          <CanvasApprovalGate
            approvalId="appr-cas-409"
            actionLabel="Execute Action"
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Execute Action/i });
        fireEvent.click(approveBtn);

        await waitFor(() => {
          const terminal = screen.getByTestId("gate-terminal");
          expect(terminal.getAttribute("data-verdict")).toBe("decided");
          expect(terminal.textContent).toContain("Action already decided.");
          expect(screen.queryByRole("button", { name: /Execute Action/i })).toBeNull();
        });
      });

      it("transitions to terminal badge on 410 expired", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 410 });
        renderWithI18n(
          <CanvasApprovalGate
            approvalId="appr-cas-410"
            actionLabel="Execute Action"
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Execute Action/i });
        fireEvent.click(approveBtn);

        await waitFor(() => {
          const terminal = screen.getByTestId("gate-terminal");
          expect(terminal.getAttribute("data-verdict")).toBe("expired");
          expect(terminal.textContent).toContain("Approval ticket expired.");
          expect(screen.queryByRole("button", { name: /Execute Action/i })).toBeNull();
        });
      });

      it("verifies fail-closed behavior on status 400 or network rejection in CanvasApprovalGate", async () => {
        const onAction = vi.fn().mockResolvedValue({ status: 400 });
        renderWithI18n(
          <CanvasApprovalGate
            approvalId="appr-fabricated-tampered"
            actionLabel="Execute Action"
            onAction={onAction}
          />,
        );

        const approveBtn = screen.getByRole("button", { name: /Execute Action/i });
        fireEvent.click(approveBtn);

        // Pre-v1.0 Fail-Closed rule: a 400 Bad Request or network error must NOT be marked as 'decided' or 'Action executed'
        await waitFor(() => {
          const terminal = screen.getByTestId("gate-terminal");
          expect(terminal.getAttribute("data-verdict")).not.toBe("decided");
        });
      });
    });

    describe("Mutating Action Cards approvalId enforcement", () => {
      it("ResumeRankCard: enforces approval-missing-id when approvalId is empty", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ResumeRankCard
            toolName="recruiting_screen"
            args={{
              job_title: "Staff Engineer",
              total_candidates: 10,
              candidates: [{ rank: 1, name: "Alice", score: 0.95 }],
              approval_id: "",
            }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const stageBtn = screen.getByTestId("stage-invites-btn");
        expect(stageBtn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(stageBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("InvoiceAuditCard: enforces approval-missing-id when approvalId is empty", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <InvoiceAuditCard
            toolName="invoice_audit"
            args={{ total_invoices: 50, matched_count: 40, approval_id: "" }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const releaseBtn = screen.getByTestId("release-payments-btn");
        expect(releaseBtn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(releaseBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("QuotationCard: enforces approval-missing-id when approvalId is empty", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <QuotationCard
            toolName="quotation_generate"
            args={{ client_name: "Acme", approval_id: "" }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const sendBtn = screen.getByTestId("send-quotation-btn");
        expect(sendBtn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(sendBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("EmailManagementCard: enforces approval-missing-id when approvalId is empty", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <EmailManagementCard
            toolName="email_manage"
            args={{ total_unread: 5, approval_id: "" }}
            onAction={onAction}
          />,
        );

        expect(screen.getByTestId("approval-missing-id")).toBeDefined();
        const sendBtn = screen.getByTestId("send-replies-btn");
        expect(sendBtn.hasAttribute("disabled")).toBe(true);
        fireEvent.click(sendBtn);
        expect(onAction).not.toHaveBeenCalled();
      });

      it("ResumeRankCard: enforces approval-missing-id when approvalId is whitespace", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <ResumeRankCard
            toolName="recruiting_screen"
            args={{
              job_title: "Staff Engineer",
              total_candidates: 10,
              candidates: [{ rank: 1, name: "Alice", score: 0.95 }],
              approval_id: "    ",
            }}
            onAction={onAction}
          />,
        );

        // Adversarial check: does whitespace bypass the missing-id check?
        const missingWarning = screen.queryByTestId("approval-missing-id");
        const stageBtn = screen.getByTestId("stage-invites-btn");
        expect(missingWarning).not.toBeNull();
        expect(stageBtn.hasAttribute("disabled")).toBe(true);
      });

      it("InvoiceAuditCard: enforces approval-missing-id when approvalId is whitespace", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <InvoiceAuditCard
            toolName="invoice_audit"
            args={{ total_invoices: 50, matched_count: 40, approval_id: "    " }}
            onAction={onAction}
          />,
        );

        const missingWarning = screen.queryByTestId("approval-missing-id");
        const releaseBtn = screen.getByTestId("release-payments-btn");
        expect(missingWarning).not.toBeNull();
        expect(releaseBtn.hasAttribute("disabled")).toBe(true);
      });

      it("QuotationCard: enforces approval-missing-id when approvalId is whitespace", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <QuotationCard
            toolName="quotation_generate"
            args={{ client_name: "Acme", approval_id: "    " }}
            onAction={onAction}
          />,
        );

        const missingWarning = screen.queryByTestId("approval-missing-id");
        const sendBtn = screen.getByTestId("send-quotation-btn");
        expect(missingWarning).not.toBeNull();
        expect(sendBtn.hasAttribute("disabled")).toBe(true);
      });

      it("EmailManagementCard: enforces approval-missing-id when approvalId is whitespace", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <EmailManagementCard
            toolName="email_manage"
            args={{ total_unread: 5, approval_id: "    " }}
            onAction={onAction}
          />,
        );

        const missingWarning = screen.queryByTestId("approval-missing-id");
        const sendBtn = screen.getByTestId("send-replies-btn");
        expect(missingWarning).not.toBeNull();
        expect(sendBtn.hasAttribute("disabled")).toBe(true);
      });

      it("SupportEscalationCard: enforces approval-missing-id when approvalId is whitespace", () => {
        const onAction = vi.fn();
        renderWithI18n(
          <SupportEscalationCard
            toolName="support_escalate"
            args={{ ticket_id: "TICK-1", approval_id: "    " }}
            onAction={onAction}
          />,
        );

        const missingWarning = screen.queryByTestId("approval-missing-id");
        const escalateBtn = screen.getByTestId("escalate-ticket-btn");
        expect(missingWarning).not.toBeNull();
        expect(escalateBtn.hasAttribute("disabled")).toBe(true);
      });
    });
  });

  // =========================================================================
  // SECTION 2: ZERO-I/O SEAM VERIFICATION
  // =========================================================================
  describe("2. Zero-I/O Seam Verification Across All Card & Canvas Files", () => {
    function getAllSourceFiles(dir: string): string[] {
      const results: string[] = [];
      const list = readdirSync(dir);
      for (const file of list) {
        const filePath = join(dir, file);
        const stat = statSync(filePath);
        if (stat.isDirectory()) {
          if (!file.includes("__tests__") && file !== "test") {
            results.push(...getAllSourceFiles(filePath));
          }
        } else if ((file.endsWith(".tsx") || file.endsWith(".ts")) && !file.includes(".test.")) {
          results.push(filePath);
        }
      }
      return results;
    }

    it("verifies zero fetch, csrfFetch, or raw /api/ across all card and canvas source files", () => {
      const copilotDir = resolve(__dirname, "../..");
      const cardsDir = resolve(copilotDir, "cards");
      const canvasDir = resolve(copilotDir, "canvas");

      const cardFiles = getAllSourceFiles(cardsDir);
      const canvasFiles = getAllSourceFiles(canvasDir);
      const allFiles = [...cardFiles, ...canvasFiles];

      expect(allFiles.length).toBeGreaterThanOrEqual(18);

      const violations: Array<{ file: string; pattern: string; snippet: string }> = [];

      for (const file of allFiles) {
        const content = readFileSync(file, "utf-8");

        const fetchMatch = content.match(/\bfetch\s*\(/);
        if (fetchMatch) {
          violations.push({ file, pattern: "fetch(", snippet: fetchMatch[0] });
        }

        const csrfMatch = content.match(/\bcsrfFetch\s*\(/);
        if (csrfMatch) {
          violations.push({ file, pattern: "csrfFetch(", snippet: csrfMatch[0] });
        }

        const apiMatch = content.match(/["']\/api\//);
        if (apiMatch) {
          violations.push({ file, pattern: "/api/", snippet: apiMatch[0] });
        }
      }

      expect(violations).toEqual([]);
    });
  });

  // =========================================================================
  // SECTION 3: ERGONOMIC STRESS & RESPONSIVE LAYOUT TRANSITIONS
  // =========================================================================
  describe("3. Ergonomic Stress & Responsive Layout Transitions", () => {
    it("preserves active draft state in WeeklyReportCanvas through rapid drawer -> split -> maximized -> drawer -> split transitions", async () => {
      renderDrawer();

      const initialArtifact: Artifact = {
        id: "art-stress-weekly",
        type: "weekly_report",
        title: "Operations Report Stress Test",
        version: 1,
        status: "draft",
        content: {
          report_title: "Operations Report Stress Test",
          markdown_report: "# Original Markdown Header",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(initialArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      // User performs in-flight edit
      const textarea = screen.getByLabelText("Markdown editor");
      fireEvent.change(textarea, { target: { value: "# In-Flight Modified Content That Must Survive Transitions" } });

      // Verify immediate update
      expect(screen.getByTestId("markdown-preview").textContent).toContain("In-Flight Modified Content That Must Survive Transitions");

      // Rapid layout cycle: split -> maximized -> split -> drawer -> split
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      expect(useCopilotStore.getState().canvasLayout).toBe("maximized");

      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });
      expect(useCopilotStore.getState().canvasLayout).toBe("split");

      act(() => {
        useCopilotStore.getState().setCanvasLayout("drawer");
      });
      expect(useCopilotStore.getState().canvasLayout).toBe("drawer");

      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });
      expect(useCopilotStore.getState().canvasLayout).toBe("split");

      // Assert draft state was NOT lost
      const currentArtifact = useCopilotStore.getState().artifacts["art-stress-weekly"];
      expect((currentArtifact.content as any).markdown_report).toContain("In-Flight Modified Content That Must Survive Transitions");
      expect(screen.getByTestId("markdown-preview").textContent).toContain("In-Flight Modified Content That Must Survive Transitions");
    });

    it("stress-tests QuotationCanvas line item addition across split -> maximized -> split transitions", async () => {
      renderDrawer();

      const initialArtifact: Artifact = {
        id: "art-stress-quote",
        type: "quotation",
        title: "Quotation Ergonomic Stress Test",
        version: 1,
        status: "draft",
        content: {
          client_name: "Acme Corp",
          line_items: [
            { sku: "ITEM-1", description: "First Baseline Item", quantity: 1, unit_price_cents: 100000 },
          ],
          approval_id: "appr-q-1",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(initialArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      // Verify 1 item originally
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(1);

      // User adds a new item
      fireEvent.click(screen.getByTestId("add-line-item-btn"));
      expect(screen.getAllByTestId("line-item-row")).toHaveLength(2);

      // Rapid layout toggle: split -> maximized -> split
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });

      // Assert that newly added item is still present after layout transition
      const rowsAfter = screen.getAllByTestId("line-item-row");
      expect(rowsAfter).toHaveLength(2);
    });

    it("stress-tests EmailDraftCanvas textarea edits across split -> maximized -> split transitions", async () => {
      renderDrawer();

      const initialArtifact: Artifact = {
        id: "art-stress-email",
        type: "email_draft",
        title: "Email Draft Ergonomic Stress Test",
        version: 1,
        status: "draft",
        content: {
          threads: [
            {
              id: "thread-1",
              sender: "client@example.com",
              subject: "Contract Inquiry",
              summary: "Client requests terms",
              draft_reply: {
                to: "client@example.com",
                subject: "Re: Contract Inquiry",
                body: "Initial unmodified draft template.",
              },
            },
          ],
          approval_id: "appr-e-1",
        },
      };

      act(() => {
        useCopilotStore.getState().openArtifact(initialArtifact);
      });

      await waitFor(() => {
        expect(screen.getByTestId("copilot-canvas-pane")).toBeDefined();
      });

      // User types custom edits into reply body textarea
      const bodyInput = screen.getByLabelText("Email reply body");
      fireEvent.change(bodyInput, { target: { value: "CRITICAL EDITED CONTENT: Special negotiated payment terms for Acme." } });
      expect((bodyInput as HTMLTextAreaElement).value).toBe("CRITICAL EDITED CONTENT: Special negotiated payment terms for Acme.");

      // Rapid layout toggle: split -> maximized -> split
      act(() => {
        useCopilotStore.getState().setCanvasLayout("maximized");
      });
      act(() => {
        useCopilotStore.getState().setCanvasLayout("split");
      });

      // Assert in-flight draft edits are preserved
      const bodyInputAfter = screen.getByLabelText("Email reply body") as HTMLTextAreaElement;
      expect(bodyInputAfter.value).toBe("CRITICAL EDITED CONTENT: Special negotiated payment terms for Acme.");
    });
  });

  // =========================================================================
  // SECTION 4: COPY & ACCESSIBILITY STRESS (ASD-STE100)
  // =========================================================================
  describe("4. Copy & Accessibility Stress (ASD-STE100 Compliance)", () => {
    it("ensures no robotic filler words appear across all card and canvas components", () => {
      const copilotDir = resolve(__dirname, "../..");
      const cardsDir = resolve(copilotDir, "cards");
      const canvasDir = resolve(copilotDir, "canvas");

      const listFiles = (dir: string): string[] => {
        const out: string[] = [];
        for (const item of readdirSync(dir)) {
          const full = join(dir, item);
          if (statSync(full).isDirectory()) {
            if (!item.includes("__tests__") && item !== "test") {
              out.push(...listFiles(full));
            }
          } else if ((item.endsWith(".tsx") || item.endsWith(".ts")) && !item.includes(".test.")) {
            out.push(full);
          }
        }
        return out;
      };

      const files = [...listFiles(cardsDir), ...listFiles(canvasDir)];
      const forbidden = [
        "delve",
        "seamlessly",
        "please",
        "obviously",
        "testament",
        "unlock the power",
        "dive in",
      ];

      const foundViolations: Array<{ file: string; word: string; line: string }> = [];

      for (const file of files) {
        const lines = readFileSync(file, "utf-8").split("\n");
        lines.forEach((line, lineNo) => {
          // ignore comments or imports
          const trimmed = line.trim();
          if (trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*")) return;

          for (const word of forbidden) {
            const regex = new RegExp(`\\b${word}\\b`, "i");
            if (regex.test(line)) {
              foundViolations.push({ file, word, line: `L${lineNo + 1}: ${line.trim()}` });
            }
          }
        });
      }

      expect(foundViolations).toEqual([]);
    });

    it("verifies user-visible sentence word count is <= 20 words for all terminal messages", () => {
      const messages = [
        "No approval ID provided. You cannot submit this action.",
        "Action approved. The change executed successfully.",
        "Action already decided. The system made no changes.",
        "Approval ticket expired. Submit a new request.",
        "Action rejected. The system made no changes.",
        "Loading data…",
        "No items to display.",
        "This capability requires an upgraded license tier.",
        "Audited 54 invoice records against purchase orders.",
        "Dispatch formal interview invitations to selected candidates.",
        "Release payment for verified invoices. The system will hold discrepant items.",
        "Send email reply to client. The system will deliver the message immediately.",
        "Escalate ticket to lead engineer. The system will dispatch an immediate notification.",
      ];

      for (const msg of messages) {
        // split by sentence boundary
        const sentences = msg.split(/[.!?]\s+/).filter(Boolean);
        for (const s of sentences) {
          const words = s.trim().split(/\s+/).filter(Boolean);
          expect(words.length).toBeLessThanOrEqual(20);
        }
      }
    });
  });
});
