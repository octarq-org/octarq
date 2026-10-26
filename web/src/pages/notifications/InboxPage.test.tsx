// @vitest-environment happy-dom
// The inbox rendered "empty" whenever `data` was empty, including when the
// query had failed. A 500 or an offline request therefore told the operator
// their inbox was empty.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../i18n";
import { InboxPage } from "./InboxPage";

const { useNotificationsQuery, refetch } = vi.hoisted(() => ({
  useNotificationsQuery: vi.fn(),
  refetch: vi.fn(),
}));

vi.mock("./api", () => ({
  useNotificationsQuery: (params: unknown) => useNotificationsQuery(params),
  useMarkReadMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useMarkAllReadMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteNotificationMutation: () => ({ mutate: vi.fn(), isPending: false }),
}));

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter>
        <InboxPage />
      </MemoryRouter>
    </I18nProvider>
  );
}

describe("InboxPage — failed load", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("shows a retryable error instead of the empty state", async () => {
    useNotificationsQuery.mockReturnValue({
      data: [],
      isLoading: false,
      isError: true,
      refetch,
      isFetching: false,
    });

    renderPage();

    expect(await screen.findByText("Could not load notifications. Try again.")).toBeTruthy();
    expect(screen.queryByText("Inbox is empty")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(refetch).toHaveBeenCalled();
  });

  it("still shows the empty state when the request succeeded", async () => {
    useNotificationsQuery.mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      refetch,
      isFetching: false,
    });

    renderPage();

    expect(await screen.findByText("Inbox is empty")).toBeTruthy();
    expect(screen.queryByText("Could not load notifications. Try again.")).toBeNull();
  });
});

describe("InboxPage — 4 horizontal filter tabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  const mockNotifs = [
    {
      id: 1,
      title: "Task completed",
      body: "Deployment succeeded",
      eventType: "task.completed",
      priority: "normal",
      readAt: null,
      createdAt: new Date().toISOString(),
    },
    {
      id: 2,
      title: "Mentioned in comment",
      body: "@operator check this out",
      eventType: "mention.user",
      priority: "normal",
      readAt: "2026-10-25T10:00:00Z",
      createdAt: new Date().toISOString(),
    },
    {
      id: 3,
      title: "High error rate",
      body: "Spike detected",
      eventType: "system.alert",
      priority: "high",
      readAt: "2026-10-25T10:00:00Z",
      createdAt: new Date().toISOString(),
    },
  ];

  it("renders 4 horizontal filter tabs and filters items accurately", async () => {
    useNotificationsQuery.mockReturnValue({
      data: mockNotifs,
      isLoading: false,
      isError: false,
      refetch,
      isFetching: false,
    });

    renderPage();

    // Verify all 4 tabs exist
    expect(screen.getByTestId("tab-unread")).toBeDefined();
    expect(screen.getByTestId("tab-all")).toBeDefined();
    expect(screen.getByTestId("tab-mentions")).toBeDefined();
    expect(screen.getByTestId("tab-system")).toBeDefined();

    // Default "all": all 3 notifications are shown
    expect(screen.getByText("Task completed")).toBeDefined();
    expect(screen.getByText("Mentioned in comment")).toBeDefined();
    expect(screen.getByText("High error rate")).toBeDefined();

    // Click Unread tab
    fireEvent.click(screen.getByTestId("tab-unread"));
    expect(screen.getByText("Task completed")).toBeDefined();
    expect(screen.queryByText("Mentioned in comment")).toBeNull();
    expect(screen.queryByText("High error rate")).toBeNull();

    // Click Mentions tab
    fireEvent.click(screen.getByTestId("tab-mentions"));
    expect(screen.queryByText("Task completed")).toBeNull();
    expect(screen.getByText("Mentioned in comment")).toBeDefined();
    expect(screen.queryByText("High error rate")).toBeNull();

    // Click System Alerts tab
    fireEvent.click(screen.getByTestId("tab-system"));
    expect(screen.queryByText("Task completed")).toBeNull();
    expect(screen.queryByText("Mentioned in comment")).toBeNull();
    expect(screen.getByText("High error rate")).toBeDefined();
  });
});

