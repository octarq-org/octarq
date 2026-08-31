// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { I18nProvider } from "../../../i18n";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import linksPlugin from "../index";
import { LinkEditorForm } from "./LinkEditorForm";
import { Link } from "../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

const existingMockLink: Link = {
  id: 101,
  host: "link.acme.com",
  slug: "spring-promo",
  title: "Spring Promo 2026",
  tags: "promo, spring, sale",
  target: "https://acme.com/spring-sale",
  note: "Internal spring sale campaign note",
  expiresAt: "2026-12-31T23:59:00.000Z",
  expiredUrl: "https://acme.com/expired-sale",
  clickLimit: 500,
  archived: false,
  enabled: true,
  clicks: 42,
  hasPassword: true,
  routingRules: [
    { type: "device", match: "mobile", target: "https://acme.com/mobile-sale" },
    { type: "split", weight: 50, target: "https://acme.com/spring-b" },
  ],
  createdAt: "2026-03-01T00:00:00Z",
};

describe("LinkEditorForm", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(linksPlugin);

    fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;

      if (method === "GET" && path === "/api/ai/assist/status") {
        return jsonResponse({ configured: true, provider: "openai" });
      }
      if (method === "POST" && path === "/api/ai/assist/suggest-slug") {
        return jsonResponse({ slugs: ["ai-suggest-1", "ai-suggest-2"] });
      }
      if (method === "GET" && path === "/api/links/metadata") {
        return jsonResponse({ title: "Fetched Page Title", description: "Desc", favicon: "" });
      }
      if (method === "POST" && path === "/api/links") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 200, ...body, clicks: 0, hasPassword: !!body.password, createdAt: new Date().toISOString() });
      }
      if (method === "PUT" && path === "/api/links/101") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ ...existingMockLink, ...body });
      }

      throw new Error(`Unhandled fetch in test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders new link form with initial state and required target validation", async () => {
    const onCancel = vi.fn();
    const onSaved = vi.fn();

    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com", "go.acme.com"]} onCancel={onCancel} onSaved={onSaved} />
      </I18nProvider>
    );

    // Verify fields are rendered
    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz");
    expect(targetInput).toBeDefined();

    const slugInput = screen.getByPlaceholderText("e.g. promo2026");
    expect(slugInput).toBeDefined();

    // Save button should be disabled when target is empty
    const saveBtn = screen.getByRole("button", { name: "Save Link" });
    expect((saveBtn as HTMLButtonElement).disabled).toBe(true);

    // Cancel button works
    const cancelBtn = screen.getByRole("button", { name: "Cancel" });
    fireEvent.click(cancelBtn);
    expect(onCancel).toHaveBeenCalledTimes(1);

    // Typing target enables save button
    fireEvent.change(targetInput, { target: { value: "https://acme.com/new-product" } });
    expect((saveBtn as HTMLButtonElement).disabled).toBe(false);
  });

  it("handles AI slug suggestions and populates slug input", async () => {
    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    // Wait for AI assist status to load
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "AI Suggest" })).toBeDefined();
    });

    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz");
    fireEvent.change(targetInput, { target: { value: "https://acme.com/awesome-tool" } });

    const aiBtn = screen.getByRole("button", { name: "AI Suggest" });
    fireEvent.click(aiBtn);

    await waitFor(() => {
      expect(screen.getByText("ai-suggest-1")).toBeDefined();
      expect(screen.getByText("ai-suggest-2")).toBeDefined();
    });

    // Clicking a suggestion chip sets the slug
    fireEvent.click(screen.getByText("ai-suggest-1"));
    const slugInput = screen.getByPlaceholderText("e.g. promo2026") as HTMLInputElement;
    expect(slugInput.value).toBe("ai-suggest-1");
  });

  it("handles AI suggest error gracefully", async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      if (urlObj.pathname === "/api/ai/assist/status") {
        return jsonResponse({ configured: true, provider: "openai" });
      }
      if (urlObj.pathname === "/api/ai/assist/suggest-slug") {
        return jsonResponse({ message: "AI rate limit" }, 500);
      }
      throw new Error(`Unhandled fetch: ${rawUrl}`);
    });

    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "AI Suggest" })).toBeDefined();
    });

    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz");
    fireEvent.change(targetInput, { target: { value: "https://acme.com/awesome-tool" } });

    const aiBtn = screen.getByRole("button", { name: "AI Suggest" });
    fireEvent.click(aiBtn);

    await waitFor(() => {
      expect(screen.getByText("AI suggestion failed")).toBeDefined();
    });
  });

  it("fetches page title metadata into title field", async () => {
    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz");
    fireEvent.change(targetInput, { target: { value: "https://acme.com/awesome-article" } });

    const fetchBtn = screen.getByRole("button", { name: "Fetch" });
    fireEvent.click(fetchBtn);

    await waitFor(() => {
      const titleInput = screen.getByPlaceholderText("Auto-populated title for page previews") as HTMLInputElement;
      expect(titleInput.value).toBe("Fetched Page Title");
    });
  });

  it("expands UTM builder and applies query parameters to destination URL", async () => {
    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz") as HTMLTextAreaElement;
    fireEvent.change(targetInput, { target: { value: "https://acme.com/landing" } });

    // Open UTM builder
    const utmBtn = screen.getByRole("button", { name: "UTM" });
    fireEvent.click(utmBtn);

    expect(screen.getByPlaceholderText("utm_source")).toBeDefined();
    expect(screen.getByPlaceholderText("utm_medium")).toBeDefined();
    expect(screen.getByPlaceholderText("utm_campaign")).toBeDefined();

    fireEvent.change(screen.getByPlaceholderText("utm_source"), { target: { value: "newsletter" } });
    fireEvent.change(screen.getByPlaceholderText("utm_medium"), { target: { value: "email" } });
    fireEvent.change(screen.getByPlaceholderText("utm_campaign"), { target: { value: "fall2026" } });
    fireEvent.change(screen.getByPlaceholderText("utm_term"), { target: { value: "discount" } });
    fireEvent.change(screen.getByPlaceholderText("utm_content"), { target: { value: "header-link" } });

    // Apply UTM
    const applyBtn = screen.getByRole("button", { name: "Apply UTM Parameters" });
    fireEvent.click(applyBtn);

    expect(targetInput.value).toBe(
      "https://acme.com/landing?utm_source=newsletter&utm_medium=email&utm_campaign=fall2026&utm_term=discount&utm_content=header-link"
    );
  });

  it("manages routing rules with add, type selection, match, target, and delete", async () => {
    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    // Add rule
    const addRuleBtn = screen.getByRole("button", { name: "+ Add Rule" });
    fireEvent.click(addRuleBtn);

    // Initial added rule is split with weight 50
    expect(screen.getByPlaceholderText("Target URL")).toBeDefined();
    expect(screen.getByText(/Remaining to control: 50%/)).toBeDefined();

    const ruleTarget = screen.getByPlaceholderText("Target URL");
    fireEvent.change(ruleTarget, { target: { value: "https://acme.com/split-target" } });

    // Add a second rule
    fireEvent.click(addRuleBtn);
    const ruleTargets = screen.getAllByPlaceholderText("Target URL");
    expect(ruleTargets.length).toBe(2);

    // Delete first rule
    const deleteButtons = screen.getAllByRole("button").filter(b => b.querySelector("svg.lucide-trash-2"));
    expect(deleteButtons.length).toBe(2);
    fireEvent.click(deleteButtons[0]);

    // Only one rule remains
    expect(screen.getAllByPlaceholderText("Target URL").length).toBe(1);
  });

  it("submits a new link with full configuration and invokes onSaved", async () => {
    let capturedPayload: any = null;
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");

      if (method === "GET" && urlObj.pathname === "/api/ai/assist/status") {
        return jsonResponse({ configured: false, provider: "" });
      }
      if (method === "POST" && urlObj.pathname === "/api/links") {
        capturedPayload = JSON.parse(init?.body as string);
        return jsonResponse({ id: 55, ...capturedPayload });
      }
      throw new Error(`Unexpected request: ${method} ${rawUrl}`);
    });

    const onSaved = vi.fn();

    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={onSaved} />
      </I18nProvider>
    );

    // Fill target
    fireEvent.change(screen.getByPlaceholderText("https://example.com/blog-post-xyz"), {
      target: { value: "https://acme.com/special-promo" },
    });

    // Fill slug
    fireEvent.change(screen.getByPlaceholderText("e.g. promo2026"), {
      target: { value: "special-slug" },
    });

    // Fill title
    fireEvent.change(screen.getByPlaceholderText("Auto-populated title for page previews"), {
      target: { value: "Special Title" },
    });

    // Fill tags
    fireEvent.change(screen.getByPlaceholderText("e.g. q3-ads, product-hunt"), {
      target: { value: "vip, promo" },
    });

    // Fill note
    fireEvent.change(screen.getByPlaceholderText("Notes visible only to team members"), {
      target: { value: "Campaign Q4" },
    });

    // Fill password
    fireEvent.change(screen.getByPlaceholderText("e.g. 123456"), {
      target: { value: "supersecret" },
    });

    // Fill redirectUrl after expiry
    fireEvent.change(screen.getByPlaceholderText("e.g. https://my-site.com/expired"), {
      target: { value: "https://acme.com/expired" },
    });

    // Submit
    const saveBtn = screen.getByRole("button", { name: "Save Link" });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledTimes(1);
    });

    expect(capturedPayload).toMatchObject({
      target: "https://acme.com/special-promo",
      slug: "special-slug",
      host: "link.acme.com",
      title: "Special Title",
      tags: "vip, promo",
      note: "Campaign Q4",
      password: "supersecret",
      expiredUrl: "https://acme.com/expired",
      enabled: true,
    });
  });

  it("pre-populates existing link data in edit mode and sends PUT request", async () => {
    let putPayload: any = null;
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");

      if (method === "GET" && urlObj.pathname === "/api/ai/assist/status") {
        return jsonResponse({ configured: false, provider: "" });
      }
      if (method === "PUT" && urlObj.pathname === "/api/links/101") {
        putPayload = JSON.parse(init?.body as string);
        return jsonResponse({ ...existingMockLink, ...putPayload });
      }
      throw new Error(`Unexpected request: ${method} ${rawUrl}`);
    });

    const onSaved = vi.fn();

    render(
      <I18nProvider>
        <LinkEditorForm
          link={existingMockLink}
          hosts={["link.acme.com", "other.acme.com"]}
          onCancel={vi.fn()}
          onSaved={onSaved}
        />
      </I18nProvider>
    );

    // Verify existing fields are filled
    const targetInput = screen.getByPlaceholderText("https://example.com/blog-post-xyz") as HTMLTextAreaElement;
    expect(targetInput.value).toBe("https://acme.com/spring-sale");

    const slugInput = screen.getByPlaceholderText("e.g. promo2026") as HTMLInputElement;
    expect(slugInput.value).toBe("spring-promo");

    const titleInput = screen.getByPlaceholderText("Auto-populated title for page previews") as HTMLInputElement;
    expect(titleInput.value).toBe("Spring Promo 2026");

    const tagsInput = screen.getByPlaceholderText("e.g. q3-ads, product-hunt") as HTMLInputElement;
    expect(tagsInput.value).toBe("promo, spring, sale");

    expect(screen.getByPlaceholderText("e.g. 123456")).toBeDefined();
    expect(screen.getByText("Optional password check")).toBeDefined();

    // Modify title and submit
    fireEvent.change(titleInput, { target: { value: "Updated Spring Promo 2026" } });
    const saveBtn = screen.getByRole("button", { name: "Save Link" });
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(onSaved).toHaveBeenCalledTimes(1);
    });

    expect(putPayload.title).toBe("Updated Spring Promo 2026");
    expect(putPayload.slug).toBe("spring-promo");
    expect(putPayload.host).toBe("link.acme.com");
  });

  it("handles 400 host is required error with localized message", async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");

      if (method === "GET" && urlObj.pathname === "/api/ai/assist/status") {
        return jsonResponse({ configured: false, provider: "" });
      }
      if (method === "POST" && urlObj.pathname === "/api/links") {
        return jsonResponse({ message: "host is required on multi-tenant instance" }, 400);
      }
      throw new Error(`Unexpected request: ${method} ${rawUrl}`);
    });

    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={[]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    fireEvent.change(screen.getByPlaceholderText("https://example.com/blog-post-xyz"), {
      target: { value: "https://acme.com/test" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Link" }));

    await waitFor(() => {
      expect(screen.getByText("Host is required: pick a link host for this short link (this instance is multi-tenant)")).toBeDefined();
    });
  });

  it("handles general API errors with FormError display", async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");

      if (method === "GET" && urlObj.pathname === "/api/ai/assist/status") {
        return jsonResponse({ configured: false, provider: "" });
      }
      if (method === "POST" && urlObj.pathname === "/api/links") {
        return jsonResponse({ message: "Slug 'custom' is already in use by another link" }, 409);
      }
      throw new Error(`Unexpected request: ${method} ${rawUrl}`);
    });

    render(
      <I18nProvider>
        <LinkEditorForm link={null} hosts={["link.acme.com"]} onCancel={vi.fn()} onSaved={vi.fn()} />
      </I18nProvider>
    );

    fireEvent.change(screen.getByPlaceholderText("https://example.com/blog-post-xyz"), {
      target: { value: "https://acme.com/test" },
    });

    fireEvent.click(screen.getByRole("button", { name: "Save Link" }));

    await waitFor(() => {
      expect(screen.getByText("Slug 'custom' is already in use by another link")).toBeDefined();
    });
  });
});
