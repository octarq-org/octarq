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
import { EmailListItem } from "./EmailListItem";
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
    const saveDraftBtn = screen.getByRole("button", { name: "Save draft" });
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

  it("renders 1-Click OTP copy bar in EmailViewForm and copies code", async () => {
    const mockEmailWithOtp: Email = {
      id: 99,
      mailboxId: 1,
      from: "security@auth.com",
      to: "user@example.com",
      subject: "Your login code is 849201",
      text: "Use code 849201 to verify.",
      html: "",
      otp: "849201",
      category: "otp",
      summary: "One-time security verification passcode.",
      importance: 5,
      read: true,
      note: "",
      attachments: "[]",
      authSpf: "pass",
      authDkim: "pass",
      authDmarc: "pass",
      receivedAt: "2026-10-24T12:00:00Z",
    };

    let copiedText = "";
    Object.defineProperty(navigator, "clipboard", {
      value: {
        writeText: vi.fn().mockImplementation(async (text: string) => {
          copiedText = text;
        }),
      },
      configurable: true,
      writable: true,
    });

    renderWithProviders(
      <EmailViewForm
        email={mockEmailWithOtp}
        onClose={vi.fn()}
        onReply={vi.fn()}
        onChanged={vi.fn()}
      />
    );

    const otpBar = screen.getByTestId("otp-copy-bar");
    expect(otpBar).toBeDefined();
    expect(screen.getByText("849201")).toBeDefined();

    const copyBtn = screen.getByRole("button", { name: "Copy Code" });
    fireEvent.click(copyBtn);

    expect(copiedText).toBe("849201");
    expect(screen.getByText("One-time security verification passcode.")).toBeDefined();
  });

  it("renders AI category badge, summary preview, and importance dots in EmailListItem", () => {
    const mockEmailItem: Email = {
      id: 55,
      mailboxId: 1,
      from: "billing@acme.com",
      to: "user@example.com",
      subject: "Monthly Subscription Invoice",
      text: "Your invoice for $49 is ready.",
      html: "",
      category: "bill",
      summary: "Monthly platform subscription invoice for $49.",
      importance: 4,
      read: false,
      note: "",
      attachments: "[]",
      authSpf: "pass",
      authDkim: "pass",
      authDmarc: "pass",
      receivedAt: "2026-10-24T10:00:00Z",
    };

    renderWithProviders(
      <EmailListItem
        email={mockEmailItem}
        folder="inbox"
        isSelected={false}
        onSelect={vi.fn()}
      />
    );

    expect(screen.getByText("Bill")).toBeDefined();
    expect(screen.getByText("Monthly platform subscription invoice for $49.")).toBeDefined();
    expect(screen.getByTestId("importance-dots")).toBeDefined();
    expect(screen.getByTestId("importance-dots").textContent).toBe("●●●●○");
  });
});
