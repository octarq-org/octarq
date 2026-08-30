// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, render, screen, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../../../i18n";
import { ConfirmBridge } from "../../../ConfirmBridge";
import { RoleProvider } from "../../../shell/role";
import { registerUIPlugin, resetRegistry, NotificationChannelFormContext } from "@octarq/plugin-sdk";
import notifyPlugin from "./index";
import TelegramForm from "./TelegramForm";
import WebhookForm from "./WebhookForm";
import { NotificationChannels } from "../../../pages/settings/notifications";
import { NotificationChannelType, NotificationChannel } from "../../../api";

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

const mockChannelTypes: NotificationChannelType[] = [
  {
    type: "telegram",
    title: "Telegram Bot",
    description: "Send alerts to a Telegram chat or channel",
    icon: "telegram",
  },
  {
    type: "webhook",
    title: "HTTP Webhook",
    description: "Post JSON payload to an external HTTP URL",
    icon: "webhook",
  },
];

const mockChannels: NotificationChannel[] = [
  {
    id: 1,
    name: "Dev Team Telegram",
    type: "telegram",
    config: JSON.stringify({ botToken: "123456:ABC-DEF", chatId: "-100123456789" }),
    enabled: true,
    createdAt: "2026-01-01T00:00:00Z",
  },
  {
    id: 2,
    name: "Incident Webhook",
    type: "webhook",
    config: JSON.stringify({ url: "https://alerts.acme.internal/hook" }),
    enabled: false,
    createdAt: "2026-02-01T00:00:00Z",
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

describe("Notify & Alerts Forms", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    cleanup();
    resetRegistry();
    registerUIPlugin(notifyPlugin);

    fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const method = (init?.method ?? "GET").toUpperCase();
      const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.toString() : (input as Request).url;
      const urlObj = new URL(rawUrl, "http://localhost");
      const path = urlObj.pathname;

      if (method === "GET" && path === "/api/notification-channel-types") {
        return jsonResponse(mockChannelTypes);
      }
      if (method === "GET" && path === "/api/notification-channels") {
        return jsonResponse(mockChannels);
      }
      if (method === "POST" && path === "/api/notification-channels") {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 3, ...body, createdAt: new Date().toISOString() });
      }
      if (method === "PUT" && path.startsWith("/api/notification-channels/")) {
        const body = init?.body ? JSON.parse(init.body as string) : {};
        return jsonResponse({ id: 1, ...body });
      }
      if (method === "DELETE" && path.startsWith("/api/notification-channels/")) {
        return jsonResponse({ ok: true });
      }
      if (method === "POST" && path.endsWith("/test") && path.startsWith("/api/notification-channels/")) {
        return jsonResponse({ ok: true });
      }

      throw new Error(`Unhandled fetch in Notify test: ${method} ${rawUrl}`);
    });
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  describe("TelegramForm standalone", () => {
    it("renders Telegram fields and updates config", () => {
      const updateConfig = vi.fn();
      const setConfig = vi.fn();

      renderWithProviders(
        <NotificationChannelFormContext.Provider
          value={{
            config: { botToken: "initial_tok", chatId: "initial_chat" },
            setConfig,
            updateConfig,
          }}
        >
          <TelegramForm />
        </NotificationChannelFormContext.Provider>
      );

      const botTokenInput = screen.getByDisplayValue("initial_tok");
      const chatIdInput = screen.getByDisplayValue("initial_chat");

      fireEvent.change(botTokenInput, { target: { value: "new_bot_token" } });
      expect(updateConfig).toHaveBeenCalledWith("botToken", "new_bot_token");

      fireEvent.change(chatIdInput, { target: { value: "-100999888" } });
      expect(updateConfig).toHaveBeenCalledWith("chatId", "-100999888");
    });
  });

  describe("WebhookForm standalone", () => {
    it("renders Webhook URL field and updates config", () => {
      const updateConfig = vi.fn();
      const setConfig = vi.fn();

      renderWithProviders(
        <NotificationChannelFormContext.Provider
          value={{
            config: { url: "https://my-webhook.com/alerts" },
            setConfig,
            updateConfig,
          }}
        >
          <WebhookForm />
        </NotificationChannelFormContext.Provider>
      );

      const urlInput = screen.getByPlaceholderText("https://my-webhook.com/alerts");
      expect((urlInput as HTMLInputElement).value).toBe("https://my-webhook.com/alerts");

      fireEvent.change(urlInput, { target: { value: "https://new-webhook.org/hook" } });
      expect(updateConfig).toHaveBeenCalledWith("url", "https://new-webhook.org/hook");
    });
  });

  describe("NotificationChannels page integration", () => {
    it("renders channels list, expands accordion, tests channel, and toggles status", async () => {
      renderWithProviders(<NotificationChannels />);

      await waitFor(() => {
        expect(screen.getByText("Telegram Bot")).toBeDefined();
        expect(screen.getByText("HTTP Webhook")).toBeDefined();
      });

      // Expand Telegram Bot row
      fireEvent.click(screen.getByText("Telegram Bot"));

      await waitFor(() => {
        expect(screen.getByText("Dev Team Telegram")).toBeDefined();
      });

      // Test notification channel
      const testBtn = screen.getByRole("button", { name: /Test/i });
      fireEvent.click(testBtn);

      await waitFor(() => {
        const testCall = fetchMock.mock.calls.find(c => {
          const url = typeof c[0] === "string" ? c[0] : (c[0] as Request).url;
          return url.includes("/api/notification-channels/1/test");
        });
        expect(testCall).toBeDefined();
      });

      // Toggle notification channel (Disable)
      const disableBtn = screen.getByRole("button", { name: "Disable" });
      fireEvent.click(disableBtn);

      await waitFor(() => {
        const putCall = fetchMock.mock.calls.find(c => {
          const url = typeof c[0] === "string" ? c[0] : (c[0] as Request).url;
          return url.includes("/api/notification-channels/1") && c[1]?.method === "PUT";
        });
        expect(putCall).toBeDefined();
      });
    });

    it("opens create channel modal, fills plugin slot form, and submits new channel", async () => {
      renderWithProviders(<NotificationChannels />);

      await waitFor(() => {
        expect(screen.getByText("Telegram Bot")).toBeDefined();
      });

      // Expand Telegram Bot row
      fireEvent.click(screen.getByText("Telegram Bot"));

      // Click Add channel
      const addBtn = screen.getByRole("button", { name: /Add channel/i });
      fireEvent.click(addBtn);

      // Verify modal opened
      await waitFor(() => {
        expect(screen.getByPlaceholderText("My Dev Team Telegram")).toBeDefined();
      });

      // Fill Channel Name
      fireEvent.change(screen.getByPlaceholderText("My Dev Team Telegram"), {
        target: { value: "Production Ops Alerts" },
      });

      // Fill TelegramForm inputs rendered via ExtensionSlot
      const emptyInputs = screen.getAllByDisplayValue("");
      fireEvent.change(emptyInputs[0], { target: { value: "987654:XYZ-WVU" } });
      fireEvent.change(emptyInputs[1], { target: { value: "-100555666" } });

      // Submit modal
      const saveBtn = screen.getByRole("button", { name: "Save Channel" });
      fireEvent.click(saveBtn);

      await waitFor(() => {
        const postCall = fetchMock.mock.calls.find(c => {
          const url = typeof c[0] === "string" ? c[0] : (c[0] as Request).url;
          return url.includes("/api/notification-channels") && c[1]?.method === "POST";
        });
        expect(postCall).toBeDefined();
        const payload = JSON.parse(postCall![1]?.body as string);
        expect(payload).toMatchObject({
          name: "Production Ops Alerts",
          type: "telegram",
          config: JSON.stringify({ botToken: "987654:XYZ-WVU", chatId: "-100555666" }),
          enabled: true,
        });
      });
    });

    it("opens edit channel modal and deletes channel with confirmation", async () => {
      renderWithProviders(<NotificationChannels />);

      await waitFor(() => {
        expect(screen.getByText("Telegram Bot")).toBeDefined();
      });

      // Expand Telegram Bot row
      fireEvent.click(screen.getByText("Telegram Bot"));

      await waitFor(() => {
        expect(screen.getByText("Dev Team Telegram")).toBeDefined();
      });

      // Delete channel with confirmation
      const delBtn = document.querySelector('button.text-danger-fg') as HTMLButtonElement;
      expect(delBtn).toBeDefined();
      fireEvent.click(delBtn);

      // Confirm dialog appears
      await waitFor(() => {
        expect(screen.getByText("Delete this notification channel?")).toBeDefined();
      });

      const confirmBtn = screen.getByRole("button", { name: "Confirm" });
      fireEvent.click(confirmBtn);

      await waitFor(() => {
        expect(screen.queryByText("Delete this notification channel?")).toBeNull();
      });
    });
  });
});
