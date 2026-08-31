// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import linksPlugin from "../index";
import LinksPage from "./index";
import { Link } from "../api";

const mockLinksWithPassword: Link[] = [
  {
    id: 1,
    host: "link.acme.com",
    slug: "pass-protected",
    title: "Protected Link",
    tags: "secret",
    target: "https://acme.com/secret",
    note: "",
    expiresAt: null,
    expiredUrl: "",
    clickLimit: 0,
    archived: false,
    enabled: true,
    clicks: 5,
    password: "SecretCode123",
    hasPassword: true,
    routingRules: [],
    createdAt: "2026-06-01T00:00:00Z",
  },
];

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

describe("Link password plaintext display and edit form", () => {
  let updatedPayload: any = null;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(linksPlugin);
    updatedPayload = null;

    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;
      if (method === "GET" && path === "/api/links") return jsonResponse(mockLinksWithPassword);
      if (method === "GET" && path === "/api/domains") return jsonResponse([]);
      if (method === "PUT" && path === "/api/links/1") {
        updatedPayload = JSON.parse((init?.body as string) || "{}");
        return jsonResponse({ ...mockLinksWithPassword[0], ...updatedPayload });
      }
      throw new Error(`unexpected request in test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  it("displays password in plaintext on the link badge and pre-fills the plaintext input in edit form", async () => {
    render(
      <MemoryRouter initialEntries={["/links"]}>
        <I18nProvider>
          <LinksPage />
        </I18nProvider>
      </MemoryRouter>
    );

    // 1. Password should be visible in plain text on the badge in the list
    await waitFor(() => {
      expect(screen.getByText("SecretCode123")).toBeDefined();
    });

    // 2. Click Edit Link to open the editor modal
    const editBtn = screen.getByTitle("Edit Link");
    fireEvent.click(editBtn);

    await waitFor(() => {
      expect(screen.getByText("Edit Link — Protected Link")).toBeDefined();
    });

    // 3. The password input field should be type="text" and contain "SecretCode123"
    const passwordInput = screen.getByDisplayValue("SecretCode123") as HTMLInputElement;
    expect(passwordInput).toBeDefined();
    expect(passwordInput.type).toBe("text");

    // 4. Save without touching password -> payload should retain the password and not silently clear it
    const saveBtn = screen.getByText("Save Link");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updatedPayload).not.toBeNull();
    });
    expect(updatedPayload.password).toBe("SecretCode123");
  });
});
