// @vitest-environment happy-dom
// A failed first page used to be swallowed (`catch` only handled 402/404), so
// `links` stayed empty and the page rendered "This workspace has no short links
// yet." over a 500 — telling the operator their links were gone.
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import linksPlugin from "../index";
import LinksPage from "./index";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={["/links"]}>
      <I18nProvider>
        <LinksPage />
      </I18nProvider>
    </MemoryRouter>
  );
}

describe("LinksPage — failed list load", () => {
  let linkStatus = 500;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(linksPlugin);
    linkStatus = 500;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const raw =
          typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
        const path = new URL(raw, "http://localhost").pathname;
        if (path === "/api/links") {
          return linkStatus >= 400
            ? jsonResponse({ message: "boom" }, linkStatus)
            : jsonResponse([]);
        }
        if (path === "/api/domains") return jsonResponse([]);
        throw new Error(`unexpected request in test: ${raw}`);
      })
    );
  });

  it("shows a retryable failure instead of the empty state", async () => {
    renderPage();

    expect(await screen.findByText("Failed to load data")).toBeTruthy();
    expect(screen.getByText("HTTP 500")).toBeTruthy();
    expect(screen.queryByText("This workspace has no short links yet.")).toBeNull();
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });

  it("recovers to the real empty state when Retry succeeds", async () => {
    renderPage();

    const retry = await screen.findByRole("button", { name: "Retry" });
    linkStatus = 200;
    fireEvent.click(retry);

    expect(await screen.findByText("This workspace has no short links yet.")).toBeTruthy();
    expect(screen.queryByText("Failed to load data")).toBeNull();
  });
});
