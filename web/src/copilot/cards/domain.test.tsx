// @vitest-environment happy-dom
import React from "react";
import { fireEvent, render as rtlRender, screen, waitFor, cleanup } from "@testing-library/react";
import { describe, expect, it, vi, afterEach } from "vitest";
import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { I18nProvider } from "../../i18n";
import type { CardAction, CardActionHandler, CardOutcome } from "./registry";
import { CardShell } from "./CardShell";
import { ApprovalCard } from "./ApprovalCard";
import { LinkCard } from "./LinkCard";
import { EmployeeCard } from "./EmployeeCard";
import { OrderCard } from "./OrderCard";
import { FinanceCard } from "./FinanceCard";
import { StatusCard } from "./StatusCard";
import { PortalCard } from "./PortalCard";

function render(ui: React.ReactElement) {
  const res = rtlRender(<I18nProvider>{ui}</I18nProvider>);
  return {
    ...res,
    rerender: (nextUi: React.ReactElement) => res.rerender(<I18nProvider>{nextUi}</I18nProvider>),
  };
}

const cardProps = (
  args: unknown,
  onAction: CardActionHandler = vi.fn(),
) => ({
  args,
  callId: "call-1",
  toolName: "test_tool",
  onAction,
});

describe("CardShell (4 States)", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders loading state with ASD-STE100 copy", () => {
    render(<CardShell title="Test Title" state="loading" />);
    expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("loading");
    expect(screen.getByText("Loading data…")).toBeDefined();
  });

  it("renders error state with detail", () => {
    render(<CardShell title="Test Title" state="error" errorDetail="Database timeout" />);
    expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("error");
    expect(screen.getByText("An error occurred. The system made no changes.")).toBeDefined();
    expect(screen.getByText(/Database timeout/)).toBeDefined();
  });

  it("renders empty state", () => {
    render(<CardShell title="Test Title" state="empty" />);
    expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("empty");
    expect(screen.getByText("No items to display.")).toBeDefined();
  });

  it("renders forbidden state (Fail-Closed Entitlement Gate)", () => {
    render(<CardShell title="Test Title" state="forbidden" forbiddenReason="Elite license required." />);
    expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("forbidden");
    expect(screen.getByText("Elite license required.")).toBeDefined();
  });

  it("renders normal ok state wrapping children", () => {
    render(
      <CardShell title="Test Title">
        <div data-testid="child-content">Child Body</div>
      </CardShell>,
    );
    expect(screen.getByTestId("card-shell").getAttribute("data-state")).toBe("ok");
    expect(screen.getByTestId("child-content")).toBeDefined();
  });
});

describe("ApprovalCard (HITL CAS & Fail-Closed Guard)", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders before and after diffs", () => {
    render(
      <ApprovalCard
        {...cardProps({
          approval_id: "appr-001",
          tool_name: "dns.records.update",
          before: { target: "1.1.1.1" },
          after: { target: "8.8.8.8" },
        })}
      />,
    );
    expect(screen.getByTestId("diff-before").textContent).toContain("1.1.1.1");
    expect(screen.getByTestId("diff-after").textContent).toContain("8.8.8.8");
  });

  it("disables submit and shows explicit warning when approval ID is missing", () => {
    render(<ApprovalCard {...cardProps({ tool_name: "risky_tool" })} />);
    const approveBtn = screen.getByRole("button", { name: /Approve/i });
    const rejectBtn = screen.getByRole("button", { name: /Reject/i });
    expect(approveBtn.hasAttribute("disabled")).toBe(true);
    expect(rejectBtn.hasAttribute("disabled")).toBe(true);
    expect(screen.getByTestId("approval-missing-id").textContent).toContain("No approval ID provided.");
  });

  it("submits approve action when approval ID is present", async () => {
    const onAction = vi.fn().mockResolvedValue({ status: 200 });
    render(<ApprovalCard {...cardProps({ approval_id: "appr-002" }, onAction)} />);
    const approveBtn = screen.getByRole("button", { name: /Approve/i });
    expect(approveBtn.hasAttribute("disabled")).toBe(false);
    fireEvent.click(approveBtn);

    expect(onAction).toHaveBeenCalledWith({
      type: "approve",
      approvalId: "appr-002",
    });

    await waitFor(() => {
      expect(screen.getByTestId("approval-terminal").getAttribute("data-verdict")).toBe("decided");
    });
  });

  it("handles 410 expired status as terminal state without repoll", async () => {
    const onAction = vi.fn().mockResolvedValue({ status: 410 });
    render(<ApprovalCard {...cardProps({ approval_id: "appr-003" }, onAction)} />);
    fireEvent.click(screen.getByRole("button", { name: /Approve/i }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-terminal").getAttribute("data-verdict")).toBe("expired");
      expect(screen.getByText(/Approval expired/)).toBeDefined();
    });
  });

  it("handles 409 conflict status as terminal decided state", async () => {
    const onAction = vi.fn().mockResolvedValue({ status: 409 });
    render(<ApprovalCard {...cardProps({ approval_id: "appr-004" }, onAction)} />);
    fireEvent.click(screen.getByRole("button", { name: /Approve/i }));

    await waitFor(() => {
      expect(screen.getByTestId("approval-terminal").getAttribute("data-verdict")).toBe("decided");
    });
  });
});

describe("LinkCard", () => {
  afterEach(() => {
    cleanup();
  });

  it("disables create button until slug and target are present", () => {
    const onAction = vi.fn();
    render(<LinkCard {...cardProps({ host: "octarq.link" }, onAction)} />);
    const createBtn = screen.getByRole("button", { name: /Create Link/i });
    expect(createBtn.hasAttribute("disabled")).toBe(true);

    fireEvent.change(screen.getByLabelText(/slug/i), { target: { value: "test" } });
    fireEvent.change(screen.getByLabelText(/target/i), { target: { value: "https://octarq.org" } });
    expect(createBtn.hasAttribute("disabled")).toBe(false);

    fireEvent.click(createBtn);
    expect(onAction).toHaveBeenCalledWith({
      type: "custom",
      payload: {
        action: "links.create",
        host: "octarq.link",
        slug: "test",
        target: "https://octarq.org",
        enabled: true,
      },
    });
  });

  it("submits links.update when editing an existing link", () => {
    const onAction = vi.fn();
    render(
      <LinkCard
        {...cardProps(
          {
            host: "octarq.link",
            slug: "existing",
            target: "https://old.com",
            enabled: true,
          },
          onAction,
        )}
      />,
    );
    const saveBtn = screen.getByRole("button", { name: /Save Changes/i });
    fireEvent.click(saveBtn);

    expect(onAction).toHaveBeenCalledWith({
      type: "custom",
      payload: {
        action: "links.update",
        host: "octarq.link",
        slug: "existing",
        target: "https://old.com",
        enabled: true,
      },
    });
  });

  it("toggles enabled status and emits custom intent", () => {
    const onAction = vi.fn();
    render(<LinkCard {...cardProps({ slug: "toggle_me", enabled: true }, onAction)} />);
    const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(true);
    fireEvent.click(checkbox);
    expect(checkbox.checked).toBe(false);

    expect(onAction).toHaveBeenCalledWith({
      type: "custom",
      payload: {
        action: "links.set_enabled",
        host: "",
        slug: "toggle_me",
        enabled: false,
      },
    });
  });

  it("copies short URL to clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    render(
      <LinkCard
        {...cardProps({
          slug: "abc",
          target: "https://example.com",
          short_url: "https://octarq.link/abc",
        })}
      />,
    );
    expect(screen.getByTestId("short-url").textContent).toContain("https://octarq.link/abc");
    const copyBtn = screen.getByRole("button", { name: /Copy/i });
    fireEvent.click(copyBtn);
    expect(writeText).toHaveBeenCalledWith("https://octarq.link/abc");
    await waitFor(() => {
      expect(screen.getByText("Copied")).toBeDefined();
    });
  });
});

describe("Domain Cards (Employee, Order, Finance, Status, Portal)", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders EmployeeCard with status and persona", () => {
    render(
      <EmployeeCard
        {...cardProps({
          id: 42,
          name: "Jarvis",
          status: "online",
          soul: "Be concise and accurate.",
          tools: ["web_search", "mail_send"],
        })}
      />,
    );
    expect(screen.getByTestId("employee-status").textContent).toContain("Online");
    expect(screen.getByTestId("employee-persona").textContent).toContain("Be concise and accurate.");
    expect(screen.getByText("web_search")).toBeDefined();
    expect(screen.getByText("mail_send")).toBeDefined();
  });

  it("renders OrderCard in get and list mode", () => {
    const { rerender } = render(
      <OrderCard {...cardProps({ orderNo: "ORD-987" })} toolName="orders_get" />,
    );
    expect(screen.getByTestId("order-no").textContent).toContain("ORD-987");

    rerender(<OrderCard {...cardProps({ limit: 10 })} toolName="orders_list" />);
    expect(screen.getByTestId("order-list").textContent).toContain("Requested latest");
  });

  it("renders FinanceCard with metrics", () => {
    render(
      <FinanceCard
        {...cardProps({
          currency: "USD",
          category: "subscription",
          kind: "charge",
          limit: 25,
        })}
        toolName="finance_list_entries"
      />,
    );
    expect(screen.getByTestId("finance-currency").textContent).toContain("USD");
    expect(screen.getByTestId("finance-category").textContent).toContain("subscription");
    expect(screen.getByTestId("finance-kind").textContent).toContain("charge");
    expect(screen.getByTestId("finance-limit").textContent).toContain("25");
  });

  it("renders StatusCard with page slug or probe", () => {
    const { rerender } = render(
      <StatusCard {...cardProps({ slug: "global" })} toolName="statuspage_public" />,
    );
    expect(screen.getByTestId("status-slug").textContent).toContain("/status/global");

    rerender(
      <StatusCard {...cardProps({ target: "api.octarq.com" })} toolName="statuspage_probe" />,
    );
    expect(screen.getByTestId("status-probe").textContent).toContain("Probe check requested");
  });

  it("renders PortalCard with buyer and license ID", () => {
    render(
      <PortalCard
        {...cardProps({ email: "operator@example.com", licenseId: 777 })}
        toolName="portal_licenses"
      />,
    );
    expect(screen.getByTestId("portal-email").textContent).toContain("operator@example.com");
    expect(screen.getByTestId("portal-license").textContent).toContain("#777");
    expect(screen.getByText(/Metadata display only/)).toBeDefined();
  });
});

describe("Zero-I/O Architectural Seam Invariant", () => {
  it("enforces zero fetch, csrfFetch, or raw /api/ in card source files", () => {
    const cardsDir = resolve(__dirname);
    const files = readdirSync(cardsDir).filter(
      (f) => (f.endsWith(".tsx") || f.endsWith(".ts")) && !f.includes(".test."),
    );

    for (const file of files) {
      const content = readFileSync(resolve(cardsDir, file), "utf-8");
      expect(content).not.toMatch(/\bfetch\s*\(/);
      expect(content).not.toMatch(/\bcsrfFetch\s*\(/);
      expect(content).not.toMatch(/["']\/api\//);
    }
  });
});
