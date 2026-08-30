// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api } from "../../api";
import { I18nProvider } from "../../i18n";
import { RoleProvider } from "../../shell/role";
import * as ui from "../../ui";
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
});
