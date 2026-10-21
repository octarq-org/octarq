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
