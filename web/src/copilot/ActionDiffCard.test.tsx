// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ActionDiffCard } from "./ActionDiffCard";
import type { ActionDiff } from "./types";
import { I18nProvider } from "../i18n";

const mockAction: ActionDiff = {
  id: "test-act-1",
  agent: "Claude Code (MCP)",
  action: "delete_dns_record",
  title: "删除 DNS 解析记录",
  target: "api.octarq.com (A -> 1.2.3.4)",
  riskLevel: "destructive",
  status: "pending",
  createdAt: "2026-09-16T10:00:00Z",
  reason: "旧版节点下线，避免解析分流。",
  diff: [
    { field: "domain", label: "目标域名", before: "api.octarq.com", after: "api.octarq.com" },
    { field: "type", label: "记录类型", before: "A", after: "(已删除)" },
    { field: "value", label: "解析值", before: "1.2.3.4", after: "(已删除)" },
  ],
};

function renderWithI18n(ui: React.ReactElement) {
  return render(<I18nProvider>{ui}</I18nProvider>);
}

describe("ActionDiffCard", () => {
  afterEach(() => {
    cleanup();
  });
  it("renders agent identity, title, target resource, and risk badge", () => {
    renderWithI18n(<ActionDiffCard action={mockAction} />);

    expect(screen.getByText("Claude Code (MCP)")).toBeDefined();
    expect(screen.getByText("删除 DNS 解析记录")).toBeDefined();
    expect(screen.getByText("api.octarq.com (A -> 1.2.3.4)")).toBeDefined();
    expect(screen.getByText("Destructive")).toBeDefined();
    expect(screen.getByText("Pending")).toBeDefined();
  });

  it("renders diff fields correctly with before and after values", () => {
    renderWithI18n(<ActionDiffCard action={mockAction} />);

    expect(screen.getByText("目标域名")).toBeDefined();
    expect(screen.getByText("记录类型")).toBeDefined();
    expect(screen.getByText("解析值")).toBeDefined();
    expect(screen.getByText("1.2.3.4")).toBeDefined();
    expect(screen.getAllByText("(已删除)").length).toBeGreaterThan(0);
  });

  it("calls onApprove callback when approve button is clicked", async () => {
    const handleApprove = vi.fn();
    renderWithI18n(<ActionDiffCard action={mockAction} onApprove={handleApprove} />);

    const approveBtn = screen.getByTitle("Approve");
    expect(approveBtn).toBeDefined();

    fireEvent.click(approveBtn);
    expect(handleApprove).toHaveBeenCalledWith("test-act-1");
  });

  it("calls onReject callback when reject button is clicked", async () => {
    const handleReject = vi.fn();
    renderWithI18n(<ActionDiffCard action={mockAction} onReject={handleReject} />);

    const rejectBtn = screen.getByTitle("Reject");
    expect(rejectBtn).toBeDefined();

    fireEvent.click(rejectBtn);
    expect(handleReject).toHaveBeenCalledWith("test-act-1");
  });

  it("renders approved state without action buttons", () => {
    const approvedAction: ActionDiff = {
      ...mockAction,
      status: "approved",
      approver: "Alex Admin",
    };

    renderWithI18n(<ActionDiffCard action={approvedAction} />);

    expect(screen.queryByTitle("Approve")).toBeNull();
    expect(screen.queryByTitle("Reject")).toBeNull();
    expect(screen.getAllByText("Approved").length).toBeGreaterThan(0);
    expect(screen.getByText(/Alex Admin/)).toBeDefined();
  });

  it("renders rejected state properly", () => {
    const rejectedAction: ActionDiff = {
      ...mockAction,
      status: "rejected",
    };

    renderWithI18n(<ActionDiffCard action={rejectedAction} />);

    expect(screen.queryByTitle("Approve")).toBeNull();
    expect(screen.getAllByText("Rejected").length).toBeGreaterThan(0);
  });
});
