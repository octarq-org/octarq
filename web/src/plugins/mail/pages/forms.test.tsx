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
import { MailboxEditor } from "./MailboxEditor";
import { SMTPSenders } from "./SMTPSenders";
import { SuppressionList } from "./SuppressionList";
import { SMTPSender } from "../../../api";
import { Mailbox, MailSuppression } from "../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

const mockSenders: SMTPSender[] = [
  {
    id: 1,
    name: "Corporate Mailgun",
    host: "smtp.mailgun.org",
    port: 587,
    user: "postmaster@acme.com",
    fromEmail: "noreply@acme.com",
    passSet: true,
    createdAt: "2026-01-01T00:00:00Z",
  },
];

const mockMailbox: Mailbox = {
  id: 10,
  address: "support@acme.com",
  note: "Customer support queue",
  enabled: true,
  unread: 3,
};

const mockSuppressions: MailSuppression[] = [
  {
    id: 1,
    address: "hardbounce@invalid.com",
    reason: "hard_bounce",
    source: "smtp",
    count: 2,
    createdAt: "2026-05-01T00:00:00Z",
    updatedAt: "2026-05-01T00:00:00Z",
  },
  {
    id: 2,
    address: "complaint@spam.com",
    reason: "complaint",
    source: "fbl",
    count: 1,
    createdAt: "2026-05-02T00:00:00Z",
    updatedAt: "2026-05-02T00:00:00Z",
  },
  {
    id: 3,
    address: "manual@blocked.com",
    reason: "manual",
    source: "admin",
    count: 1,
    createdAt: "2026-05-03T00:00:00Z",
    updatedAt: "2026-05-03T00:00:00Z",
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

describe("Mail Forms", () => {
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
        return jsonResponse(mockSenders);
      }
      if (method === "POST" && path === "/api/smtp-senders") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 2, ...body, passSet: !!body.pass, createdAt: new Date().toISOString() });
      }
      if (method === "PUT" && path.startsWith("/api/smtp-senders/")) {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 1, ...body, passSet: true });
      }
      if (method === "DELETE" && path.startsWith("/api/smtp-senders/")) {
        return jsonResponse({ ok: true });
      }
      if (method === "POST" && path.endsWith("/test") && path.startsWith("/api/smtp-senders/")) {
        return jsonResponse({ ok: true });
      }

      if (method === "GET" && path === "/api/settings") {
        return jsonResponse({ autoWrapLinks: true });
      }

      if (method === "POST" && path === "/api/emails/send") {
        return jsonResponse({ ok: true });
      }

      if (method === "GET" && path === "/api/mailboxes") {
        return jsonResponse([mockMailbox]);
      }
      if (method === "POST" && path === "/api/mailboxes") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 11, ...body, unread: 0 });
      }
      if (method === "PUT" && path === "/api/mailboxes/10") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ ...mockMailbox, ...body });
      }
      if (method === "DELETE" && path === "/api/mailboxes/10") {
        return jsonResponse({ ok: true });
      }

      if (method === "GET" && path === "/api/mail/suppressions") {
        return jsonResponse(mockSuppressions);
      }
      if (method === "POST" && path === "/api/mail/suppressions") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({
          id: 4,
          address: body.address,
          reason: "manual",
          source: "admin",
          count: 1,
          createdAt: new Date().toISOString(),
          updatedAt: new Date().toISOString(),
        });
      }
      if (method === "DELETE" && path.startsWith("/api/mail/suppressions/")) {
        return jsonResponse({ ok: true });
      }

      throw new Error(`Unhandled fetch in Mail test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  describe("Compose", () => {
    it("renders compose modal, validates required fields, and sends email successfully", async () => {
      const onClose = vi.fn();

      renderWithProviders(<Compose onClose={onClose} />);

      // Verify fields
      expect(screen.getByText("Compose Mail")).toBeDefined();
      expect((screen.getByRole("button", { name: "Send Mail" }) as HTMLButtonElement).disabled).toBe(true);

      // Wait for settings & senders to load and checkbox to appear
      await waitFor(() => {
        expect(screen.getByRole("checkbox")).toBeDefined();
      });

      // Fill in recipients, subject, body
      const toInput = screen.getByPlaceholderText("hello@world.com");
      fireEvent.change(toInput, {
        target: { value: "alice@example.com, bob@example.com" },
      });
      const subjectInput = screen.getByPlaceholderText("Subject line");
      fireEvent.change(subjectInput, {
        target: { value: "Important System Update" },
      });
      fireEvent.change(screen.getByPlaceholderText("Type mail content here..."), {
        target: { value: "Hello team,\nPlease read https://example.com/update" },
      });
      fireEvent.change(screen.getByPlaceholderText("e.g. custom@domain.com"), {
        target: { value: "team@acme.com" },
      });

      // Submit email
      const sendBtn = screen.getByRole("button", { name: "Send Mail" });
      expect((sendBtn as HTMLButtonElement).disabled).toBe(false);
      fireEvent.click(sendBtn);

      await waitFor(() => {
        expect(screen.getByText("Message Sent Successfully")).toBeDefined();
      });

      // Verify the POST payload sent to /api/emails/send
      const sendCall = fetchMock.mock.calls.find(c => {
        const url = typeof c[0] === "string" ? c[0] : (c[0] as Request).url;
        return url.includes("/api/emails/send");
      });
      expect(sendCall).toBeDefined();
      const payload = JSON.parse(sendCall![1]?.body as string);
      expect(payload).toMatchObject({
        to: ["alice@example.com", "bob@example.com"],
        from: "team@acme.com",
        subject: "Important System Update",
        text: "Hello team,\nPlease read https://example.com/update",
        smtpSenderId: 1,
        trackLinks: true,
      });

      // Click Done
      fireEvent.click(screen.getByRole("button", { name: "Done" }));
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("cancels compose when cancel button is clicked", async () => {
      const onClose = vi.fn();
      renderWithProviders(<Compose onClose={onClose} />);

      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      expect(onClose).toHaveBeenCalledTimes(1);
    });

    it("pre-fills draft reply data", async () => {
      renderWithProviders(
        <Compose
          draft={{ to: "customer@example.com", subject: "Re: Order #1234" }}
          onClose={vi.fn()}
        />
      );

      const toInput = screen.getByPlaceholderText("hello@world.com") as HTMLInputElement;
      expect(toInput.value).toBe("customer@example.com");

      const subjectInput = screen.getByPlaceholderText("Subject line") as HTMLInputElement;
      expect(subjectInput.value).toBe("Re: Order #1234");
    });

    it("displays FormError when sending email fails", async () => {
      fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
        const method = (init?.method ?? "GET").toUpperCase();
        const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
        const urlObj = new URL(rawUrl, "http://localhost");
        const path = urlObj.pathname;

        if (method === "GET" && path === "/api/smtp-senders") return jsonResponse(mockSenders);
        if (method === "GET" && path === "/api/settings") return jsonResponse({});
        if (method === "POST" && path === "/api/emails/send") {
          return jsonResponse({ message: "SMTP relay authentication failed" }, 400);
        }
        throw new Error(`Unhandled fetch: ${method} ${rawUrl}`);
      });

      renderWithProviders(<Compose onClose={vi.fn()} />);

      expect(screen.getByText("Compose Mail")).toBeDefined();

      fireEvent.change(screen.getByPlaceholderText("hello@world.com"), { target: { value: "test@example.com" } });
      fireEvent.change(screen.getByPlaceholderText("Subject line"), { target: { value: "Test" } });

      const sendBtn = screen.getByRole("button", { name: "Send Mail" });
      expect((sendBtn as HTMLButtonElement).disabled).toBe(false);
      fireEvent.click(sendBtn);

      await waitFor(() => {
        expect(screen.getByText("SMTP relay authentication failed")).toBeDefined();
      });
    });
  });

  describe("MailboxEditor", () => {
    it("renders new mailbox form, validates prefix & domain, and creates mailbox", async () => {
      const onSaved = vi.fn();
      const onClose = vi.fn();

      renderWithProviders(
        <MailboxEditor
          box={null}
          hosts={["acme.com", "mail.acme.com"]}
          onClose={onClose}
          onSaved={onSaved}
        />
      );

      expect(screen.getByText("Create Mailbox")).toBeDefined();
      const prefixInput = screen.getByPlaceholderText("e.g. sales");
      const noteInput = screen.getByPlaceholderText("e.g. support operations");

      fireEvent.change(prefixInput, { target: { value: "hello" } });
      fireEvent.change(noteInput, { target: { value: "General contact inbox" } });

      const saveBtn = screen.getByRole("button", { name: "Save Configuration" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(onSaved).toHaveBeenCalledTimes(1);
      });
    });

    it("renders warning when no mail hosts are configured", async () => {
      renderWithProviders(
        <MailboxEditor
          box={null}
          hosts={[]}
          onClose={vi.fn()}
          onSaved={vi.fn()}
        />
      );

      expect(screen.getByText("No mail-enabled hosts. Configure your custom domain first.")).toBeDefined();
    });

    it("renders edit mailbox mode, updates note/enabled, and deletes mailbox with confirmation", async () => {
      const onSaved = vi.fn();

      renderWithProviders(
        <MailboxEditor
          box={mockMailbox}
          hosts={["acme.com"]}
          onClose={vi.fn()}
          onSaved={onSaved}
        />
      );

      expect(screen.getByText("Edit Mailbox")).toBeDefined();
      const addressInput = screen.getByDisplayValue("support@acme.com") as HTMLInputElement;
      expect(addressInput.disabled).toBe(true);

      const noteInput = screen.getByPlaceholderText("e.g. support operations") as HTMLTextAreaElement;
      expect(noteInput.value).toBe("Customer support queue");

      fireEvent.change(noteInput, { target: { value: "Updated support queue note" } });

      const saveBtn = screen.getByRole("button", { name: "Save Configuration" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(onSaved).toHaveBeenCalledTimes(1);
      });

      // Delete mailbox with confirmation
      const delBtn = screen.getByRole("button", { name: "Delete Mailbox Completely" });
      fireEvent.click(delBtn);

      await waitFor(() => {
        expect(screen.getByText("Delete mailbox support@acme.com and all its email messages?")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Delete mailbox support@acme.com and all its email messages?")).toBeNull();
      });
    });
  });

  describe("SMTPSenders", () => {
    it("renders senders, tests connection, creates new SMTP relay, and removes relay", async () => {
      renderWithProviders(<SMTPSenders />);

      await waitFor(() => {
        expect(screen.getByText("Corporate Mailgun")).toBeDefined();
        expect(screen.getByText("noreply@acme.com via smtp.mailgun.org:587")).toBeDefined();
        expect(screen.getByText("password set")).toBeDefined();
      });

      // Test connection
      const testBtn = screen.getByRole("button", { name: "Test" });
      fireEvent.click(testBtn);

      await waitFor(() => {
        expect(screen.getByRole("button", { name: "Test" })).toBeDefined();
      });

      // Click + Add SMTP
      const addBtn = screen.getByRole("button", { name: "+ Add SMTP" });
      fireEvent.click(addBtn);

      await waitFor(() => {
        expect(screen.getByText("Add SMTP Sender")).toBeDefined();
      });

      fireEvent.change(screen.getByPlaceholderText("e.g. Corporate SMTP"), {
        target: { value: "SendGrid Gateway" },
      });
      fireEvent.change(screen.getByPlaceholderText("smtp.mailgun.org"), {
        target: { value: "smtp.sendgrid.net" },
      });
      fireEvent.change(screen.getByPlaceholderText("587"), {
        target: { value: "465" },
      });
      fireEvent.change(screen.getByPlaceholderText("e.g. postmaster@domain.com"), {
        target: { value: "apikey" },
      });
      fireEvent.change(screen.getByPlaceholderText("••••••••"), {
        target: { value: "SG.secret_token_123" },
      });
      fireEvent.change(screen.getByPlaceholderText("noreply@domain.com"), {
        target: { value: "alerts@acme.com" },
      });

      const saveBtn = screen.getByRole("button", { name: "Save" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(screen.queryByText("Add SMTP Sender")).toBeNull();
      });

      // Edit SMTP relay
      const editBtn = screen.getByRole("button", { name: "Edit SMTP" });
      fireEvent.click(editBtn);

      await waitFor(() => {
        expect(screen.getByText("Edit SMTP Sender")).toBeDefined();
      });
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      await waitFor(() => {
        expect(screen.queryByText("Edit SMTP Sender")).toBeNull();
      });

      // Remove SMTP relay
      const removeBtn = screen.getByRole("button", { name: "Remove" });
      fireEvent.click(removeBtn);

      await waitFor(() => {
        expect(screen.getByText("Remove this SMTP sender config? Outbound mail relying on it will fail to send.")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Remove this SMTP sender config? Outbound mail relying on it will fail to send.")).toBeNull();
      });
    });
  });

  describe("SuppressionList", () => {
    it("renders suppression list with reason badges, adds new suppression, and removes item", async () => {
      renderWithProviders(<SuppressionList />);

      await waitFor(() => {
        expect(screen.getByText("hardbounce@invalid.com")).toBeDefined();
        expect(screen.getByText("complaint@spam.com")).toBeDefined();
        expect(screen.getByText("manual@blocked.com")).toBeDefined();
        expect(screen.getByText("Hard Bounce")).toBeDefined();
        expect(screen.getByText("Complaint")).toBeDefined();
        expect(screen.getByText("Manual")).toBeDefined();
      });

      // Add address
      const addBtn = screen.getByRole("button", { name: "Add Address" });
      fireEvent.click(addBtn);

      expect(screen.getByText("Add to Suppression List")).toBeDefined();

      const addrInput = screen.getByPlaceholderText("colleague@example.com");
      fireEvent.change(addrInput, { target: { value: "spammer@bad.org" } });

      const saveBtn = screen.getByRole("button", { name: "Save Configuration" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        expect(screen.queryByText("Add to Suppression List")).toBeNull();
      });

      // Remove suppression item with confirmation
      const delButtons = screen.getAllByTitle("Remove");
      fireEvent.click(delButtons[0]);

      await waitFor(() => {
        expect(screen.getByText("Remove hardbounce@invalid.com from suppression list?")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Remove hardbounce@invalid.com from suppression list?")).toBeNull();
      });
    });
  });
});
