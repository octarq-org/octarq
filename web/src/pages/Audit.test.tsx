// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, cleanup } from "@testing-library/react";
import AuditLogPage from "./Audit";
import { api, AuditLog } from "../api";
import { I18nProvider } from "../i18n";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("../api", () => ({
  api: {
    auditLogs: vi.fn(),
  },
}));

function renderWithProviders(ui: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 0,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>{ui}</I18nProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
});

describe("AuditLogPage with ProTable", () => {
  const mockLogs: AuditLog[] = [
    {
      id: 1,
      orgId: 1,
      actorId: 10,
      action: "domain.create",
      targetType: "domain",
      targetId: 101,
      meta: '{"domain":"example.com"}',
      ip: "192.168.1.1",
      createdAt: "2026-09-08T12:00:00Z",
    },
    {
      id: 2,
      orgId: 1,
      actorId: 0,
      action: "cert.delete",
      targetType: "cert",
      targetId: 202,
      meta: "automated renewal expired",
      ip: "127.0.0.1",
      createdAt: "2026-09-08T11:00:00Z",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders audit logs table with formatted columns", async () => {
    vi.mocked(api.auditLogs).mockResolvedValue(mockLogs);

    renderWithProviders(<AuditLogPage />);

    // Check page header
    expect(screen.getByRole("heading", { level: 1 })).toBeDefined();

    // Check audit rows rendered
    await waitFor(() => {
      expect(screen.getByText("domain.create")).toBeDefined();
      expect(screen.getByText("cert.delete")).toBeDefined();
      expect(screen.getByText("#101")).toBeDefined();
      expect(screen.getByText("#202")).toBeDefined();
      expect(screen.getByText("192.168.1.1")).toBeDefined();
      expect(screen.getByText("127.0.0.1")).toBeDefined();
    });

    expect(api.auditLogs).toHaveBeenCalledTimes(1);
  });

  it("filters audit logs by action through ProTable search", async () => {
    vi.mocked(api.auditLogs).mockResolvedValue(mockLogs);

    renderWithProviders(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.getByText("domain.create")).toBeDefined();
    });

    // Find action search input
    const inputs = screen.getAllByRole("textbox");
    const actionInput = inputs[0]; // first filter input
    fireEvent.change(actionInput, { target: { value: "domain.create" } });

    // Submit search
    const searchBtn = screen.getByRole("button", { name: /查询|Search/i });
    fireEvent.click(searchBtn);

    await waitFor(() => {
      expect(api.auditLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({
          action: "domain.create",
          limit: 10,
          offset: 0,
        })
      );
    });
  });
});
