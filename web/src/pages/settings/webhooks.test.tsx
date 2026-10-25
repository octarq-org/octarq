// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api } from "../../api";
import { I18nProvider } from "../../i18n";
import { RoleProvider } from "../../shell/role";
import * as ui from "@octarq/plugin-sdk";
import { WebhooksSettings } from "./webhooks";

function renderWebhooksSettings(role = "admin", isInstanceAdmin = false) {
  return render(
    <I18nProvider>
      <RoleProvider value={{ role, isInstanceAdmin }}>
        <WebhooksSettings />
      </RoleProvider>
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("WebhooksSettings suite", () => {
  it("renders loading skeleton while webhooks request is pending and hides empty state", () => {
    vi.spyOn(api, "webhooks").mockReturnValue(new Promise(() => {}));
    vi.spyOn(api, "webhookEvents").mockReturnValue(new Promise(() => {}));

    renderWebhooksSettings("admin", false);

    expect(document.querySelector('[aria-busy="true"]')).toBeTruthy();
    expect(screen.getByRole("status", { name: /loading webhooks/i })).toBeTruthy();
    expect(screen.queryByText(/no outbound webhooks configured/i)).toBeNull();
  });

  it("renders error alert and retry button on 500 failure and does not render empty state", async () => {
    vi.spyOn(api, "webhooks").mockRejectedValue(new Error("Request failed with status 500"));
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);

    renderWebhooksSettings("admin", false);

    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.getByRole("button", { name: /retry/i })).toBeTruthy();
    expect(screen.getByText(/could not load webhooks/i)).toBeTruthy();
    expect(screen.queryByText(/no outbound webhooks configured/i)).toBeNull();
  });

  it("retrying after failure re-loads and recovers when request succeeds", async () => {
    const webhooksSpy = vi.spyOn(api, "webhooks")
      .mockRejectedValueOnce(new Error("Request failed with status 500"))
      .mockResolvedValueOnce([
        {
          id: 1,
          name: "Webhook 1",
          url: "https://example.com/webhook",
          events: "*",
          enabled: true,
          createdAt: "2026-08-31T00:00:00Z",
          updatedAt: "2026-08-31T00:00:00Z",
        },
      ]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);

    renderWebhooksSettings("admin", false);

    const retryBtn = await screen.findByRole("button", { name: /retry/i });
    expect(screen.queryByText(/no outbound webhooks configured/i)).toBeNull();

    fireEvent.click(retryBtn);

    expect(await screen.findByText("Webhook 1")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(screen.queryByText(/no outbound webhooks configured/i)).toBeNull();
    expect(webhooksSpy).toHaveBeenCalledTimes(2);
  });

  it("renders webhook list and handles empty state", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);

    renderWebhooksSettings("admin", false);

    expect(await screen.findByText(/no outbound webhooks configured/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /add webhook/i })).toBeTruthy();
  });

  it("creates a new webhook with all events (*)", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    const createSpy = vi.spyOn(api, "createWebhook").mockResolvedValue({
      id: 10,
      name: "Slack notifier",
      url: "https://hooks.slack.com/services/123",
      secret: "whsec_test",
      events: "*",
      enabled: true,
      createdAt: "2026-08-31T00:00:00Z",
      updatedAt: "2026-08-31T00:00:00Z",
    });
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderWebhooksSettings("admin", false);

    fireEvent.click(await screen.findByRole("button", { name: /add webhook/i }));

    expect(screen.getByText("Add Webhook Endpoint")).toBeTruthy();
    const nameInput = screen.getByPlaceholderText("n8n automation");
    fireEvent.change(nameInput, { target: { value: "Slack notifier" } });

    const urlInput = screen.getByPlaceholderText("https://your-server.com/webhooks/octarq");
    fireEvent.change(urlInput, { target: { value: "https://hooks.slack.com/services/123" } });

    const secretInput = screen.getByPlaceholderText("Custom signing secret");
    fireEvent.change(secretInput, { target: { value: "whsec_test" } });

    fireEvent.click(screen.getByRole("button", { name: /^add$/i }));

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        name: "Slack notifier",
        url: "https://hooks.slack.com/services/123",
        secret: "whsec_test",
        events: "*",
        enabled: true,
      });
      expect(toastSpy).toHaveBeenCalled();
      expect(screen.getByText("Slack notifier")).toBeTruthy();
    });
  });

  it("toggles webhook enabled status", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([
      {
        id: 1,
        name: "Discord hook",
        url: "https://discord.com/api/webhooks/1",
        events: "*",
        enabled: true,
        createdAt: "2026-08-31T00:00:00Z",
        updatedAt: "2026-08-31T00:00:00Z",
      },
    ]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    const updateSpy = vi.spyOn(api, "updateWebhook").mockResolvedValue({
      id: 1,
      name: "Discord hook",
      url: "https://discord.com/api/webhooks/1",
      events: "*",
      enabled: false,
      createdAt: "2026-08-31T00:00:00Z",
      updatedAt: "2026-08-31T00:00:00Z",
    });

    renderWebhooksSettings("admin", false);

    expect(await screen.findByText("Discord hook")).toBeTruthy();
    const toggleBtn = screen.getByRole("switch");
    fireEvent.click(toggleBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(1, { enabled: false });
    });
  });

  it("tests a webhook endpoint", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([
      {
        id: 1,
        name: "Discord hook",
        url: "https://discord.com/api/webhooks/1",
        events: "*",
        enabled: true,
        createdAt: "2026-08-31T00:00:00Z",
        updatedAt: "2026-08-31T00:00:00Z",
      },
    ]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    const testSpy = vi.spyOn(api, "testWebhook").mockResolvedValue({ ok: true });
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderWebhooksSettings("admin", false);

    const testBtn = await screen.findByRole("button", { name: /test/i });
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith(1);
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("deletes a webhook after confirmation dialog", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([
      {
        id: 1,
        name: "Discord hook",
        url: "https://discord.com/api/webhooks/1",
        events: "*",
        enabled: true,
        createdAt: "2026-08-31T00:00:00Z",
        updatedAt: "2026-08-31T00:00:00Z",
      },
    ]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const deleteSpy = vi.spyOn(api, "deleteWebhook").mockResolvedValue(undefined as any);

    renderWebhooksSettings("admin", false);

    const deleteBtn = await screen.findByRole("button", { name: /delete/i });
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(deleteSpy).toHaveBeenCalledWith(1);
    });
  });

  it("switches to Inbound Relays tab and displays relays and dead-letter queue", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    vi.spyOn(api, "webhookRelayEndpoints").mockResolvedValue([
      {
        id: "relay-stripe",
        name: "Stripe Billing & Checkout Relay",
        path: "/api/webhooks/inbound/stripe",
        algorithm: "HMAC-SHA256",
        status: "Verified & Active",
        description: "Processes payment intents, subscription updates, and checkout sessions.",
      },
      {
        id: "relay-github",
        name: "GitHub Repository Relay",
        path: "/api/webhooks/inbound/github",
        algorithm: "HMAC-SHA256",
        status: "Verified & Active",
        description: "Receives release events, commit pushes, and repository alerts.",
      },
    ]);
    vi.spyOn(api, "webhookRelayDeadLetters").mockResolvedValue([
      {
        id: 101,
        deliveryId: "del_01jf8b2c4e9a1k7m",
        event: "mail.receive",
        url: "https://ops-relay.internal.net/events/inbound",
        attempts: 5,
        responseCode: 504,
        lastError: "Gateway Timeout: upstream host failed to respond within 15 seconds.",
        createdAt: "18m ago",
      },
    ]);

    renderWebhooksSettings("admin", false);

    const inboundTab = screen.getByTestId("tab-inbound-relays");
    fireEvent.click(inboundTab);

    expect(screen.getByTestId("inbound-relays-panel")).toBeTruthy();
    expect(screen.getByText("Inbound Webhook Relays")).toBeTruthy();
    expect(await screen.findByText("Stripe Billing & Checkout Relay")).toBeTruthy();
    expect(await screen.findByText("GitHub Repository Relay")).toBeTruthy();
    expect(screen.getByText("Dead-Letter Queue (DLQ) Audit")).toBeTruthy();
    expect(await screen.findByText(/del_01jf8b2c4e9a1k7m/)).toBeTruthy();
  });

  it("replays a dead-letter queue entry and displays success toast", async () => {
    vi.spyOn(api, "webhooks").mockResolvedValue([]);
    vi.spyOn(api, "webhookEvents").mockResolvedValue([]);
    vi.spyOn(api, "webhookRelayEndpoints").mockResolvedValue([]);
    vi.spyOn(api, "webhookRelayDeadLetters").mockResolvedValue([
      {
        id: 101,
        deliveryId: "del_01jf8b2c4e9a1k7m",
        event: "mail.receive",
        url: "https://ops-relay.internal.net/events/inbound",
        attempts: 5,
        responseCode: 504,
        lastError: "Gateway Timeout: upstream host failed to respond within 15 seconds.",
        createdAt: "18m ago",
      },
    ]);
    vi.spyOn(api, "replayWebhookDeadLetter").mockResolvedValue({ ok: true });
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderWebhooksSettings("admin", false);

    fireEvent.click(screen.getByTestId("tab-inbound-relays"));

    const replayButtons = await screen.findAllByRole("button", { name: /replay/i });
    expect(replayButtons.length).toBeGreaterThan(0);

    fireEvent.click(replayButtons[0]);

    await waitFor(() => {
      expect(toastSpy).toHaveBeenCalledWith(expect.stringMatching(/queued for replay/i));
    });
  });
});
