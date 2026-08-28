// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import linksPlugin from "../index";
import LinksPage from "./index";
import { Link, LinkStats } from "../api";

const mockLinks: Link[] = [
  {
    id: 1,
    host: "link.acme.com",
    slug: "summer-sale",
    title: "Summer Sale 2026",
    tags: "ecommerce, promo, vip",
    target: "https://acme.com/summer-sale",
    note: "Summer promo link",
    expiresAt: null,
    expiredUrl: "",
    clickLimit: 0,
    archived: false,
    enabled: true,
    clicks: 88,
    hasPassword: false,
    routingRules: [],
    createdAt: "2026-06-01T00:00:00Z",
  },
  {
    id: 2,
    host: "",
    slug: "random-slug-without-title",
    title: "",
    tags: "",
    target: "https://acme.com/about",
    note: "",
    expiresAt: null,
    expiredUrl: "",
    clickLimit: 0,
    archived: false,
    enabled: true,
    clicks: 12,
    hasPassword: false,
    routingRules: [],
    createdAt: "2026-06-02T00:00:00Z",
  },
];

const mockStats: LinkStats = {
  total: 88,
  windowed: 40,
  days: 30,
  metric: "uv",
  series: [{ key: "2026-06-01", count: 10 }],
  referers: [],
  channels: [],
  countries: [],
  regions: [],
  devices: [],
  browsers: [],
  utmSources: [],
  utmMediums: [],
  utmCampaigns: [],
  variants: [],
};

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

describe("LinksPage title priority and tag badges", () => {
  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(linksPlugin);

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;
      if (method === "GET" && path === "/api/links") return jsonResponse(mockLinks);
      if (method === "GET" && path.startsWith("/api/links/1/stats")) return jsonResponse(mockStats);
      if (method === "GET" && path.startsWith("/api/links/2/stats")) return jsonResponse(mockStats);
      if (method === "GET" && path === "/api/domains") return jsonResponse([]);
      throw new Error(`unexpected request in test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  it("renders title as primary, slug as secondary, and tags split into individual badges", async () => {
    render(
      <MemoryRouter initialEntries={["/links"]}>
        <I18nProvider>
          <LinksPage />
        </I18nProvider>
      </MemoryRouter>
    );

    // Wait for links to load
    await waitFor(() => {
      expect(screen.getByText("Summer Sale 2026")).toBeDefined();
    });

    // Primary title has font-medium and text-foreground
    const title = screen.getByText("Summer Sale 2026");
    expect(title.className).toContain("font-medium");
    expect(title.className).toContain("text-foreground");

    // Secondary short slug
    expect(screen.getByText("link.acme.com/")).toBeDefined();
    expect(screen.getByText("summer-sale")).toBeDefined();

    // Tags are rendered as separate badges, not one single concatenated string
    const tagEcommerce = screen.getByTitle("ecommerce");
    const tagPromo = screen.getByTitle("promo");
    const tagVip = screen.getByTitle("vip");
    expect(tagEcommerce).toBeDefined();
    expect(tagPromo).toBeDefined();
    expect(tagVip).toBeDefined();
    expect(tagEcommerce.textContent).toContain("ecommerce");
    expect(tagPromo.textContent).toContain("promo");
    expect(tagVip.textContent).toContain("vip");

    // Link without title falls back to slug in primary title and secondary slug
    const untitledMatches = screen.getAllByText("random-slug-without-title");
    expect(untitledMatches.length).toBe(2);
  });

  it("displays link title in Edit, Analytics, and QR modals with slug fallback", async () => {
    render(
      <MemoryRouter initialEntries={["/links"]}>
        <I18nProvider>
          <LinksPage />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Summer Sale 2026")).toBeDefined();
    });

    // Test Analytics modal title for link with title
    const analyticsButtons = screen.getAllByTitle("Analytics");
    fireEvent.click(analyticsButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Click Performance Analytics — Summer Sale 2026")).toBeDefined();
    });

    // Close Analytics modal
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    // Test QR modal title for link with title
    const qrButtons = screen.getAllByTitle("QR Code");
    fireEvent.click(qrButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Link QR Code — Summer Sale 2026")).toBeDefined();
    });

    // Close QR modal
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    // Test Edit modal title for link with title
    const editButtons = screen.getAllByTitle("Edit Link");
    fireEvent.click(editButtons[0]);

    await waitFor(() => {
      expect(screen.getByText("Edit Link — Summer Sale 2026")).toBeDefined();
    });

    // Close Edit modal
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    // Test Edit modal title for link without title (fallback to /slug)
    fireEvent.click(editButtons[1]);

    await waitFor(() => {
      expect(screen.getByText("Edit Link — /random-slug-without-title")).toBeDefined();
    });
  });
});
