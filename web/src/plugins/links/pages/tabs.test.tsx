// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import LinksPage from "./index";
import { linksApi } from "../api";
import { api } from "../../../api";

vi.mock("../api", () => ({
  linksApi: {
    links: vi.fn(),
    updateLink: vi.fn(),
    deleteLink: vi.fn(),
  },
}));

vi.mock("../../../api", () => ({
  api: {
    domains: vi.fn(),
  },
  effectiveLinkHosts: (d: any) => [d.domain],
}));

vi.mock("../../../shell/role", () => ({
  useCurrentRole: () => ({ role: "admin", isInstanceAdmin: true }),
  roleSatisfies: () => true,
}));

function renderPage() {
  return render(
    <I18nProvider>
      <MemoryRouter>
        <LinksPage />
      </MemoryRouter>
    </I18nProvider>
  );
}

describe("LinksPage — 3 Horizontal Projection Tabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (api.domains as any).mockResolvedValue([{ id: 1, domain: "go.link" }]);
    (linksApi.links as any).mockResolvedValue([
      {
        id: 1,
        slug: "docs",
        target: "https://docs.example.com",
        clicks: 42,
        active: true,
        archived: false,
        host: "go.link",
      },
    ]);
  });

  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("renders 3 horizontal projection tabs and switches between them cleanly", async () => {
    renderPage();

    // Verify all 3 tabs are present
    expect(screen.getByTestId("tab-links")).toBeDefined();
    expect(screen.getByTestId("tab-anomalies")).toBeDefined();
    expect(screen.getByTestId("tab-reserved")).toBeDefined();

    // Default projection: Links view
    await waitFor(() => {
      expect(screen.getByTestId("links-projection-view")).toBeDefined();
    });

    // Switch to Anomaly Monitoring projection
    fireEvent.click(screen.getByTestId("tab-anomalies"));
    expect(screen.getByTestId("anomalies-projection-view")).toBeDefined();
    expect(screen.queryByTestId("links-projection-view")).toBeNull();
    expect(screen.getByText("Traffic Anomalies")).toBeDefined();
    expect(screen.getByText("Bot Filtering")).toBeDefined();

    // Switch to Reserved Words projection
    fireEvent.click(screen.getByTestId("tab-reserved"));
    expect(screen.getByTestId("reserved-projection-view")).toBeDefined();
    expect(screen.queryByTestId("anomalies-projection-view")).toBeNull();
    expect(screen.getByText("System Reserved Words")).toBeDefined();
    expect(screen.getByText("/api")).toBeDefined();
    expect(screen.getByText("/instance")).toBeDefined();

    // Switch back to Links projection
    fireEvent.click(screen.getByTestId("tab-links"));
    expect(screen.getByTestId("links-projection-view")).toBeDefined();
  });
});
