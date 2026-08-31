// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { ConfirmBridge } from "../../../ConfirmBridge";
import { RoleProvider } from "../../../shell/role";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
import mailPlugin from "../index";
import { Compose } from "./Compose";
import { EmailViewForm } from "./EmailView";
import { FolderNav } from "./FolderNav";
import { Email } from "../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

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

describe("Mail Enhancements", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(mailPlugin);

    fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;

      if (method === "GET" && path === "/api/smtp-senders") {
        return jsonResponse([]);
      }
      if (method === "GET" && path === "/api/settings") {
        return jsonResponse({});
      }
      if (method === "GET" && path === "/api/mail/contacts") {
        return jsonResponse([
          { id: 1, address: "contact@example.com", name: "Key Contact", interactionCount: 5, lastSeenAt: "2026-08-31T00:00:00Z" },
        ]);
      }
      if (method === "POST" && path === "/api/mail/drafts") {
        return jsonResponse({
          id: 99,
          to: "draft@example.com",
          subject: "Saved Draft Subject",
          text: "Saved body",
          folder: "drafts",
          read: true,
          receivedAt: new Date().toISOString(),
        });
      }
      if (method === "GET" && path === "/api/ai-assist/status") {
        return jsonResponse({ configured: false });
      }

      throw new Error(`Unhandled fetch in test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders unsubscribe button and handles click in EmailViewForm", async () => {
    const mockEmail: Email = {
      id: 42,
      mailboxId: 1,
      from: "newsletter@promo.com",
      to: "user@example.com",
      subject: "Special Offer",
      text: "Check out this offer!",
      html: "",
      unsubscribeUrl: "https://promo.com/unsub/token123",
      read: true,
      note: "",
      attachments: "[]",
      authSpf: "pass",
      authDkim: "pass",
      authDmarc: "pass",
      receivedAt: "2026-08-31T10:00:00Z",
    };

    const windowOpenSpy = vi.spyOn(window, "open").mockImplementation(() => null);

    renderWithProviders(
      <EmailViewForm
        email={mockEmail}
        onClose={vi.fn()}
        onReply={vi.fn()}
        onChanged={vi.fn()}
      />
    );

    const unsubButtons = screen.getAllByRole("button", { name: "Unsubscribe" });
    expect(unsubButtons.length).toBeGreaterThan(0);
    fireEvent.click(unsubButtons[0]);

    expect(windowOpenSpy).toHaveBeenCalledWith("https://promo.com/unsub/token123", "_blank", "noopener,noreferrer");
  });

  it("saves draft and selects contacts in Compose", async () => {
    const onDraftSaved = vi.fn();
    renderWithProviders(
      <Compose
        onClose={vi.fn()}
        onDraftSaved={onDraftSaved}
      />
    );

    // Typing in To should load contact suggestions
    const toInput = screen.getByPlaceholderText("hello@world.com");
    fireEvent.focus(toInput);

    await waitFor(() => {
      expect(screen.getByText("Key Contact <contact@example.com>")).toBeDefined();
    });

    fireEvent.mouseDown(screen.getByText("Key Contact <contact@example.com>"));
    expect((toInput as HTMLInputElement).value).toBe("contact@example.com");

    // Click Save Draft
    const saveDraftBtn = screen.getByRole("button", { name: "Save Draft" });
    fireEvent.click(saveDraftBtn);

    await waitFor(() => {
      expect(onDraftSaved).toHaveBeenCalledTimes(1);
    });
  });

  it("switches folders in FolderNav", async () => {
    const onSelect = vi.fn();
    renderWithProviders(
      <FolderNav currentFolder="inbox" onSelectFolder={onSelect} />
    );

    expect(screen.getByText("Inbox")).toBeDefined();
    expect(screen.getByText("Sent")).toBeDefined();
    expect(screen.getByText("Drafts")).toBeDefined();
    expect(screen.getByText("Trash")).toBeDefined();
    expect(screen.getByText("Spam")).toBeDefined();

    fireEvent.click(screen.getByText("Drafts"));
    expect(onSelect).toHaveBeenCalledWith("drafts");
  });
});
