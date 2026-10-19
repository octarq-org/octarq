// @vitest-environment happy-dom
// The overview used to swallow a failed fetch (`catch(() => {})`), leaving `o`
// null — which the page renders as a skeleton. An outage therefore looked like
// an endless load. A failure must be visible and retryable.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../i18n";
import OverviewPage from "./Overview";
import { fetchOverview, api } from "../api";

vi.mock("./dashboard/components/HealthOverview", () => ({ HealthOverview: () => null }));
vi.mock("./dashboard/components/ActivityFeed", () => ({ ActivityFeed: () => null }));

vi.mock("../api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api")>();
  return {
    ...actual,
    fetchOverview: vi.fn(),
    api: {
      ...actual.api,
      twoFAStatus: vi.fn().mockResolvedValue({ enabled: false }),
      tokens: vi.fn().mockResolvedValue([]),
      getUserSettings: vi.fn().mockResolvedValue({}),
    },
  };
});

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter>
        <OverviewPage />
      </MemoryRouter>
    </I18nProvider>
  );
}

const emptyOverview = {
  tokens: 1,
  links: 2,
  activeLinks: 1,
  domains: 1,
  mailboxes: 1,
  emails: 0,
  unread: 0,
  totalClicks: 10,
  clicks7d: 5,
  clicks30d: 8,
  series: [],
  topLinks: [],
  devices: [],
  countries: [],
  cities: [],
  recentEmails: [],
};

describe("Overview — failed load", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("shows an error with retry instead of an endless skeleton", async () => {
    (fetchOverview as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("boom"));

    renderPage();

    expect(await screen.findByText("Failed to load the overview.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
    expect(document.querySelector('[aria-busy="true"]')).toBeNull();
  });

  it("leaves the error state when Retry succeeds", async () => {
    (fetchOverview as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error("boom"))
      .mockResolvedValueOnce(emptyOverview);

    renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));

    await waitFor(() => expect(fetchOverview).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.queryByText("Failed to load the overview.")).toBeNull()
    );
  });
});
