// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { I18nProvider } from "../../i18n";
import { req } from "../../api";
import DataPage from "./index";
import type { Catalog, DataList } from "./types";

vi.mock("../../api", () => ({ req: vi.fn() }));

const catalog: Catalog = {
  entities: [
    {
      slug: "tickets",
      label: "Ticket",
      pluralLabel: "Tickets",
      icon: "ticket",
      fields: [
        {
          name: "title",
          type: "string",
          label: "Title",
          widget: "text",
          required: true,
          unique: true,
          listColumn: true,
        },
        {
          name: "closed",
          type: "boolean",
          label: "Closed",
          widget: "checkbox",
          required: false,
          unique: false,
          listColumn: true,
        },
      ],
      operations: ["list", "get", "create", "update", "delete"],
    },
  ],
};

const rows: DataList = {
  items: [{ id: "t1", title: "Printer is down", closed: false, created_at: "2026-11-14T02:00:00Z" }],
  total: 1,
  limit: 25,
  offset: 0,
};

function renderPage(slug: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <MemoryRouter initialEntries={[`/data/${slug}`]}>
          <Routes>
            <Route path="/data/:slug" element={<DataPage />} />
          </Routes>
        </MemoryRouter>
      </I18nProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("DataPage", () => {
  it("renders the declared columns and the stored rows", async () => {
    vi.mocked(req).mockImplementation(async (_method, path) =>
      path === "/api/data" ? catalog : rows,
    );

    renderPage("tickets");

    expect(await screen.findByRole("heading", { name: "Tickets" })).toBeTruthy();
    expect(await screen.findByText("Printer is down")).toBeTruthy();
    // The boolean column renders through the label dictionary, not "false".
    expect(screen.getByText("No")).toBeTruthy();
    // create is granted, so the page offers it.
    expect(screen.getByRole("button", { name: /New record/ })).toBeTruthy();
  });

  it("asks for a page of rows with the limit and offset the table picked", async () => {
    vi.mocked(req).mockImplementation(async (_method, path) =>
      path === "/api/data" ? catalog : rows,
    );

    renderPage("tickets");

    await screen.findByText("Printer is down");
    const listCall = vi.mocked(req).mock.calls.find(([, path]) => path.startsWith("/api/data/"));
    expect(listCall?.[1]).toContain("limit=");
    expect(listCall?.[1]).toContain("offset=0");
  });

  it("answers a slug the catalog does not declare with a neutral note", async () => {
    vi.mocked(req).mockImplementation(async () => catalog);

    renderPage("unknown");

    expect(await screen.findByText("This page is not available.")).toBeTruthy();
  });

  it("hides the create action when the caller may not create", async () => {
    const readOnly: Catalog = {
      entities: [{ ...catalog.entities[0], operations: ["list", "get"] }],
    };
    vi.mocked(req).mockImplementation(async (_method, path) =>
      path === "/api/data" ? readOnly : rows,
    );

    renderPage("tickets");

    await screen.findByText("Printer is down");
    expect(screen.queryByRole("button", { name: /New record/ })).toBeNull();
  });
});
