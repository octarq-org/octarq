// @vitest-environment happy-dom
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ActivityFeed, ACTIVITY_QUERY_KEY, fetchActivityFeed } from "./ActivityFeed";
import { api } from "../../../api";
import { I18nProvider } from "../../../i18n";

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
    },
  });
}

function renderWithProviders(ui: React.ReactElement, client = createTestQueryClient()) {
  return render(
    <MemoryRouter>
      <QueryClientProvider client={client}>
        <I18nProvider>{ui}</I18nProvider>
      </QueryClientProvider>
    </MemoryRouter>
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("fetchActivityFeed", () => {
  it("merges notifications and audit logs newest-first with a cap", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue([
      { id: 1, userId: "u", orgId: "1", eventType: "mail.inbound", title: "New mail", body: "hi", priority: "normal", readAt: null, createdAt: "2026-09-21T02:00:00Z" },
      { id: 2, userId: "u", orgId: "1", eventType: "system.notice", title: "Old note", body: "", priority: "normal", readAt: "2026-09-20T00:00:00Z", createdAt: "2026-09-19T00:00:00Z" },
    ]);
    vi.spyOn(api, "auditLogs").mockResolvedValue([
      { id: 7, orgId: 1, actorId: 1, action: "link.create", targetType: "link", targetId: 9, meta: "", ip: "", createdAt: "2026-09-21T01:00:00Z" },
    ]);
    const cards = await fetchActivityFeed();
    expect(cards.map((c) => c.key)).toEqual(["notification-1", "audit-7", "notification-2"]);
    expect(cards[0].unread).toBe(true);
    expect(cards[0].target).toBe("/notifications");
    expect(cards[1].target).toBe("/audit");
  });

  it("degrades to empty when both sources fail", async () => {
    vi.spyOn(api, "notifications").mockRejectedValue(new Error("down"));
    vi.spyOn(api, "auditLogs").mockRejectedValue(new Error("down"));
    await expect(fetchActivityFeed()).resolves.toEqual([]);
  });
});

describe("ActivityFeed", () => {
  it("renders cards and an empty state", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue([
      { id: 1, userId: "u", orgId: "1", eventType: "mail.inbound", title: "New mail", body: "OTP inside", priority: "normal", readAt: null, createdAt: "2026-09-21T02:00:00Z" },
    ]);
    vi.spyOn(api, "auditLogs").mockResolvedValue([]);
    const client = createTestQueryClient();
    renderWithProviders(<ActivityFeed />, client);
    await waitFor(() => expect(screen.getByText("New mail")).toBeTruthy());
    expect(screen.getByText("OTP inside")).toBeTruthy();
  });

  it("renders the empty state when there is no activity", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue([]);
    vi.spyOn(api, "auditLogs").mockResolvedValue([]);
    renderWithProviders(<ActivityFeed />);
    await waitFor(() => expect(screen.getByText("No recent activity")).toBeTruthy());
  });
});
