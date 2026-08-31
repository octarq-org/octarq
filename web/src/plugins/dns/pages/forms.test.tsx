// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { ConfirmBridge } from "../../../ConfirmBridge";
import { RoleProvider } from "../../../shell/role";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import domainsPlugin from "../index";
import { DomainEditorForm } from "./DomainEditorForm";
import { RecordsView } from "./RecordsView";
import { DDNSView } from "./DDNSView";
import { ProviderAccounts } from "./ProviderAccounts";
import { Domain, ProviderAccount } from "../../../api";
import { DNSRecord, DDNSToken } from "../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

const mockProviderAccounts: ProviderAccount[] = [
  {
    id: 1,
    name: "Cloudflare Production",
    type: "cloudflare",
    config: {},
    hasCredentials: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
  {
    id: 2,
    name: "DNSPod Secondary",
    type: "dnspod",
    config: {},
    hasCredentials: false,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
  },
];

const mockDomain: Domain = {
  id: 10,
  name: "example.com",
  providerAccountId: 1,
  zoneId: "zone_cf_123",
  note: "Main corporate domain",
  forMail: true,
  forLink: true,
  linkHosts: [{ host: "go.example.com", enabled: true }],
  mailHosts: [{ host: "mail.example.com", enabled: true }],
  createdAt: "2026-01-10T00:00:00Z",
};

const mockRecords: DNSRecord[] = [
  {
    id: "rec_1",
    type: "A",
    name: "example.com",
    content: "192.0.2.1",
    ttl: 1,
    proxied: true,
    comment: "Apex web server",
  },
  {
    id: "rec_2",
    type: "CNAME",
    name: "go.example.com",
    content: "example.com",
    ttl: 1,
    proxied: true,
    comment: "Shortlink proxy",
  },
  {
    id: "rec_3",
    type: "MX",
    name: "example.com",
    content: "mail.example.com",
    ttl: 300,
    proxied: false,
    comment: "Inbound mail server",
    priority: 10,
  },
];

const mockDDNSTokens: DDNSToken[] = [
  {
    id: 5,
    domainId: 10,
    recordName: "home.example.com",
    recordType: "A",
    label: "Home Router",
    lastIp: "203.0.113.42",
    lastSeenAt: "2026-06-01T12:00:00Z",
    createdAt: "2026-05-01T00:00:00Z",
  },
];

function renderWithProviders(ui: React.ReactElement) {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <RoleProvider value={{ role: "admin", isInstanceAdmin: true }}>
          <ConfirmBridge>{ui}</ConfirmBridge>
        </RoleProvider>
      </I18nProvider>
    </MemoryRouter>
  );
}

describe("DNS Forms", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(domainsPlugin);

    fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;

      if (method === "GET" && path === "/api/dns/providers") {
        return jsonResponse(["cloudflare", "dnspod", "route53", "aliyun"]);
      }
      if (method === "GET" && path === "/api/provider-accounts") {
        return jsonResponse(mockProviderAccounts);
      }
      if (method === "POST" && path === "/api/provider-accounts") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 3, ...body, hasCredentials: !!body.config, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() });
      }
      if (method === "PUT" && path.startsWith("/api/provider-accounts/")) {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 1, ...body, updatedAt: new Date().toISOString() });
      }
      if (method === "DELETE" && path.startsWith("/api/provider-accounts/")) {
        return jsonResponse({ ok: true });
      }

      if (method === "POST" && path === "/api/domains") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 11, ...body, forMail: false, forLink: false, createdAt: new Date().toISOString() });
      }
      if (method === "PUT" && path === "/api/domains/10") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ ...mockDomain, ...body });
      }
      if (method === "GET" && path === "/api/domains/10/records") {
        return jsonResponse(mockRecords);
      }
      if (method === "POST" && path === "/api/domains/10/records") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: "rec_new", ...body });
      }
      if (method === "PUT" && path === "/api/domains/10/records/rec_1") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ ...mockRecords[0], ...body });
      }
      if (method === "DELETE" && path === "/api/domains/10/records/rec_1") {
        return jsonResponse({ ok: true });
      }

      if (method === "GET" && path === "/api/dns/ddns") {
        return jsonResponse(mockDDNSTokens);
      }
      if (method === "POST" && path === "/api/dns/ddns") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({
          id: 6,
          ...body,
          secret: "ddns_sec_abcdef123456",
          updateUrl: `/api/dns/ddns/update?token=ddns_sec_abcdef123456`,
          createdAt: new Date().toISOString(),
        });
      }
      if (method === "DELETE" && path.startsWith("/api/dns/ddns/")) {
        return jsonResponse({ ok: true });
      }

      throw new Error(`Unhandled fetch in DNS test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  describe("DomainEditorForm", () => {
    it("renders new domain creation form with validation and submit", async () => {
      const onCancel = vi.fn();
      const onSaved = vi.fn();

      renderWithProviders(
        <DomainEditorForm
          domain={null}
          accounts={mockProviderAccounts}
          onCancel={onCancel}
          onSaved={onSaved}
        />
      );

      const nameInput = screen.getByPlaceholderText("example.com") as HTMLInputElement;
      expect(nameInput).toBeDefined();

      const saveBtn = screen.getByRole("button", { name: "Save Basic Info" }) as HTMLButtonElement;
      expect(saveBtn.disabled).toBe(true);

      // Cancel button
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(onCancel).toHaveBeenCalledTimes(1);

      // Fill form
      fireEvent.change(nameInput, { target: { value: "mynewdomain.org" } });
      expect(saveBtn.disabled).toBe(false);

      const zoneInput = screen.getByPlaceholderText("Auto-discovered if using Cloudflare");
      fireEvent.change(zoneInput, { target: { value: "zone_cf_999" } });

      const noteInput = screen.getByPlaceholderText("Optional note for team members");
      fireEvent.change(noteInput, { target: { value: "Production domain note" } });

      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(onSaved).toHaveBeenCalledTimes(1);
      });
      expect(onSaved.mock.calls[0][0]).toMatchObject({
        name: "mynewdomain.org",
        zoneId: "zone_cf_999",
        note: "Production domain note",
      });
    });

    it("renders edit domain mode with disabled domain name and saves update", async () => {
      const onSaved = vi.fn();

      renderWithProviders(
        <DomainEditorForm
          domain={mockDomain}
          accounts={mockProviderAccounts}
          onCancel={vi.fn()}
          onSaved={onSaved}
        />
      );

      const nameInput = screen.getByPlaceholderText("example.com") as HTMLInputElement;
      expect(nameInput.value).toBe("example.com");
      expect(nameInput.disabled).toBe(true);

      const noteInput = screen.getByPlaceholderText("Optional note for team members") as HTMLTextAreaElement;
      expect(noteInput.value).toBe("Main corporate domain");

      fireEvent.change(noteInput, { target: { value: "Updated note" } });

      const saveBtn = screen.getByRole("button", { name: "Save Basic Info" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(onSaved).toHaveBeenCalledTimes(1);
      });
    });

    it("displays FormError when domain save fails", async () => {
      fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
        const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
        const urlObj = new URL(rawUrl, "http://localhost");
        if (urlObj.pathname === "/api/domains") {
          return jsonResponse({ message: "Domain already registered" }, 409);
        }
        throw new Error(`Unhandled: ${rawUrl}`);
      });

      const onSaved = vi.fn();

      renderWithProviders(
        <DomainEditorForm
          domain={null}
          accounts={mockProviderAccounts}
          onCancel={vi.fn()}
          onSaved={onSaved}
        />
      );

      fireEvent.change(screen.getByPlaceholderText("example.com"), { target: { value: "existing.com" } });
      fireEvent.click(screen.getByRole("button", { name: "Save Basic Info" }));

      await waitFor(() => {
        expect(screen.getByText("Domain already registered")).toBeDefined();
      });
    });
  });

  describe("RecordsView", () => {
    it("renders records table, filters records, and opens custom record modal", async () => {
      renderWithProviders(<RecordsView domain={mockDomain} />);

      await waitFor(() => {
        expect(screen.getByText("192.0.2.1")).toBeDefined();
        expect(screen.getByText("Shortlink proxy")).toBeDefined();
        expect(screen.getByText("Inbound mail server")).toBeDefined();
      });

      // Filter by search query
      const searchInput = screen.getByPlaceholderText("Filter name / content / comment…");
      fireEvent.change(searchInput, { target: { value: "192.0.2.1" } });

      await waitFor(() => {
        expect(screen.getByText("192.0.2.1")).toBeDefined();
        expect(screen.queryByText("Shortlink proxy")).toBeNull();
      });

      // Clear search
      fireEvent.change(searchInput, { target: { value: "" } });

      // Open custom record modal
      const customBtn = screen.getByRole("button", { name: "+ Custom" });
      fireEvent.click(customBtn);

      expect(screen.getByText("Create Record")).toBeDefined();
      expect(screen.getByPlaceholderText("@ or subdomain")).toBeDefined();

      // Fill custom record form
      fireEvent.change(screen.getByPlaceholderText("@ or subdomain"), { target: { value: "api" } });
      const contentInput = document.querySelector('input[required]') as HTMLInputElement;
      expect(contentInput).toBeDefined();
      fireEvent.change(contentInput, { target: { value: "198.51.100.10" } });

      fireEvent.change(screen.getByPlaceholderText("e.g. DNS verified token or note"), {
        target: { value: "API gateway" },
      });

      // Save custom record
      fireEvent.click(screen.getByRole("button", { name: "Save Record" }));

      await waitFor(() => {
        expect(screen.queryByText("Create Record")).toBeNull();
      });
    });

    it("opens preset configurator and applies Link CNAME preset", async () => {
      renderWithProviders(<RecordsView domain={mockDomain} />);

      await waitFor(() => {
        expect(screen.getByText("192.0.2.1")).toBeDefined();
      });

      // Open preset modal
      const presetBtn = screen.getByRole("button", { name: "+ Preset" });
      fireEvent.click(presetBtn);

      expect(screen.getByText("Preset Configurator")).toBeDefined();
      expect(screen.getByRole("button", { name: "Set Link CNAME" })).toBeDefined();
      expect(screen.getByRole("button", { name: "Set MX records" })).toBeDefined();

      // Click Set Link CNAME
      fireEvent.click(screen.getByRole("button", { name: "Set Link CNAME" }));

      const commentInput = screen.getByPlaceholderText("e.g. DNS verified token or note") as HTMLInputElement;
      expect(commentInput.value).toBe("octarq short-link host");

      // Cancel modal
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(screen.queryByText("Preset Configurator")).toBeNull();
    });

    it("edits an existing record and deletes a record with confirmation", async () => {
      renderWithProviders(<RecordsView domain={mockDomain} />);

      await waitFor(() => {
        expect(screen.getByText("192.0.2.1")).toBeDefined();
      });

      // Edit record
      const editButtons = screen.getAllByRole("button", { name: "Edit" });
      fireEvent.click(editButtons[0]);

      expect(screen.getByText("Modify Record")).toBeDefined();
      const contentInputs = document.querySelectorAll('input');
      const targetInput = Array.from(contentInputs).find(i => i.value === "192.0.2.1");
      expect(targetInput).toBeDefined();

      fireEvent.change(targetInput!, { target: { value: "192.0.2.200" } });
      fireEvent.click(screen.getByRole("button", { name: "Save Record" }));

      await waitFor(() => {
        expect(screen.queryByText("Modify Record")).toBeNull();
      });

      // Delete record with confirmation
      const delButtons = screen.getAllByRole("button", { name: "Delete" });
      fireEvent.click(delButtons[0]);

      // Confirm dialog appears
      await waitFor(() => {
        expect(screen.getByText("Delete A example.com?")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Delete A example.com?")).toBeNull();
      });
    });
  });

  describe("DDNSView", () => {
    it("renders DDNS tokens, creates new token with secret reveal, and revokes token", async () => {
      renderWithProviders(<DDNSView domains={[mockDomain]} />);

      await waitFor(() => {
        expect(screen.getByText("Home Router")).toBeDefined();
        expect(screen.getByText("home.example.com")).toBeDefined();
        expect(screen.getByText("203.0.113.42")).toBeDefined();
      });

      // Click + New DDNS Token
      const newBtn = screen.getByRole("button", { name: "New DDNS Token" });
      fireEvent.click(newBtn);

      expect(screen.getByPlaceholderText("e.g. home.example.com")).toBeDefined();

      fireEvent.change(screen.getByPlaceholderText("e.g. home.example.com"), {
        target: { value: "nas.example.com" },
      });
      fireEvent.change(screen.getByPlaceholderText("e.g. Home Router"), {
        target: { value: "NAS Server" },
      });

      // Submit token creation form
      const genBtn = screen.getByRole("button", { name: "Generate Token" });
      fireEvent.click(genBtn);

      // Secret modal should open
      await waitFor(() => {
        expect(screen.getByText("DDNS Token Created")).toBeDefined();
        expect(screen.getByText("Copy your secret token now. It will never be displayed again!")).toBeDefined();
      });

      const secretInput = screen.getByDisplayValue("ddns_sec_abcdef123456");
      expect(secretInput).toBeDefined();

      // Close secret modal
      const doneBtn = screen.getByRole("button", { name: "Done" });
      fireEvent.click(doneBtn);

      expect(screen.queryByText("DDNS Token Created")).toBeNull();

      // Revoke token with confirm
      const revokeBtn = screen.getByRole("button", { name: "Revoke Token" });
      fireEvent.click(revokeBtn);

      await waitFor(() => {
        expect(screen.getByText("Are you sure you want to revoke this DDNS token? Devices using it will fail to update DNS.")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Are you sure you want to revoke this DDNS token? Devices using it will fail to update DNS.")).toBeNull();
      });
    });
  });

  describe("ProviderAccounts", () => {
    it("renders provider accounts, creates a new provider account, and removes account", async () => {
      renderWithProviders(<ProviderAccounts />);

      await waitFor(() => {
        expect(screen.getByText("Cloudflare Production")).toBeDefined();
        expect(screen.getByText("DNSPod Secondary")).toBeDefined();
        expect(screen.getByText("credentials set")).toBeDefined();
        expect(screen.getByText("no credentials")).toBeDefined();
      });

      // Open Add Provider modal
      const addBtn = screen.getByRole("button", { name: "+ Add Provider" });
      fireEvent.click(addBtn);

      expect(screen.getByText("Add DNS Provider")).toBeDefined();

      fireEvent.change(screen.getByPlaceholderText("e.g. Acme Production DNS"), {
        target: { value: "Cloudflare Staging" },
      });

      const keyInput = screen.getByPlaceholderText("API token...");
      fireEvent.change(keyInput, { target: { value: "cf_token_staging_12345" } });

      const submitBtn = screen.getByRole("button", { name: "Save" });
      fireEvent.click(submitBtn);

      await waitFor(() => {
        expect(screen.queryByText("Add DNS Provider")).toBeNull();
      });

      // Edit provider account
      const editButtons = screen.getAllByRole("button", { name: "Edit" });
      fireEvent.click(editButtons[0]);

      expect(screen.getByText("Edit DNS Provider")).toBeDefined();
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(screen.queryByText("Edit DNS Provider")).toBeNull();
      });

      // Remove provider account
      const removeButtons = screen.getAllByRole("button", { name: "Remove" });
      fireEvent.click(removeButtons[1]);

      await waitFor(() => {
        expect(screen.getByText("Remove this provider account? Managed domains using these credentials will fail sync.")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Remove this provider account? Managed domains using these credentials will fail sync.")).toBeNull();
      });
    });
  });
});
