// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { z } from "zod";
import { api } from "../../api";
import { I18nProvider } from "../../i18n";
import { parseWithFallback } from "../../lib/parseWithFallback";
import { NotificationBell } from "./NotificationBell";
import { InboxDrawer } from "./InboxDrawer";
import { InboxPage } from "./InboxPage";
import { PreferencesPage } from "./PreferencesPage";
import { useNotificationUIStore } from "./store";
import { NotificationItem, RegisteredChannel } from "./schemas";

function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function renderWithProviders(ui: React.ReactElement, queryClient = createTestQueryClient()) {
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <MemoryRouter>{ui}</MemoryRouter>
      </I18nProvider>
    </QueryClientProvider>,
  );
}

const mockNotifications: NotificationItem[] = [
  {
    id: 1,
    userId: "1",
    orgId: "1",
    eventType: "security.login_alert",
    title: "New sign-in from unusual IP",
    body: "Sign in from 192.168.1.100 was detected.",
    priority: "high",
    readAt: null,
    createdAt: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
  },
  {
    id: 2,
    userId: "1",
    orgId: "1",
    eventType: "system.upgrade",
    title: "System Update v1.2",
    body: "Database migrations finished successfully.",
    priority: "normal",
    readAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
    createdAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
  },
];

const mockChannels: RegisteredChannel[] = [
  { name: "in_app", displayName: "In-App Inbox" },
  { name: "email", displayName: "Email Service" },
  { name: "telegram", displayName: "Telegram Bot" },
];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  useNotificationUIStore.setState({ isDrawerOpen: false, filter: "all", selectedId: null });
});

describe("Runtime Type Safety & parseWithFallback", () => {
  it("passes through valid data matching Zod schema", () => {
    const schema = z.object({ count: z.number() });
    const res = parseWithFallback(schema, { count: 42 }, { count: 0 });
    expect(res).toEqual({ count: 42 });
  });

  it("safely falls back without throwing when external API response is corrupt", () => {
    const schema = z.object({ count: z.number() });
    const fallback = { count: 0 };
    const res = parseWithFallback(schema, { count: "not-a-number" }, fallback);
    expect(res).toEqual(fallback);
  });
});

describe("NotificationBell", () => {
  it("renders notification bell and shows unread badge when unread notifications exist", async () => {
    vi.spyOn(api, "notifications").mockImplementation(async (opts) => {
      if (opts?.unreadOnly) {
        return mockNotifications.filter((n) => !n.readAt);
      }
      return mockNotifications;
    });

    renderWithProviders(<NotificationBell />);

    // 1 unread notification in mockNotifications
    const badge = await screen.findByTestId("notification-unread-badge");
    expect(badge).toBeTruthy();
    expect(badge.textContent).toBe("1");
  });

  it("toggles drawer state when clicking bell button", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue([]);

    renderWithProviders(<NotificationBell />);

    const bellBtn = screen.getByRole("button", { name: /站内通知|Notifications/i });
    expect(useNotificationUIStore.getState().isDrawerOpen).toBe(false);

    fireEvent.click(bellBtn);
    expect(useNotificationUIStore.getState().isDrawerOpen).toBe(true);

    fireEvent.click(bellBtn);
    expect(useNotificationUIStore.getState().isDrawerOpen).toBe(false);
  });
});

describe("InboxDrawer", () => {
  beforeEach(() => {
    useNotificationUIStore.setState({ isDrawerOpen: true, filter: "all" });
  });

  it("renders notification items and unread indicator inside drawer", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);

    renderWithProviders(<InboxDrawer />);

    expect(await screen.findByText("New sign-in from unusual IP")).toBeTruthy();
    expect(screen.getByText("System Update v1.2")).toBeTruthy();
    expect(screen.getByTestId("unread-dot")).toBeTruthy();
  });

  it("triggers mark single as read mutation", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);
    const markSpy = vi.spyOn(api, "markNotificationRead").mockResolvedValue({ ok: true });

    renderWithProviders(<InboxDrawer />);

    const markBtn = await screen.findByRole("button", { name: /标记已读|Mark read/i });
    fireEvent.click(markBtn);

    await waitFor(() => {
      expect(markSpy).toHaveBeenCalledWith(1);
    });
  });

  it("triggers mark all as read mutation", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);
    const markAllSpy = vi.spyOn(api, "markAllNotificationsRead").mockResolvedValue({ ok: true });

    renderWithProviders(<InboxDrawer />);

    await screen.findByText("New sign-in from unusual IP");
    const markAllBtn = screen.getByRole("button", { name: /全部已读|Mark all/i });
    fireEvent.click(markAllBtn);

    await waitFor(() => {
      expect(markAllSpy).toHaveBeenCalled();
    });
  });

  it("triggers delete notification", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);
    const deleteSpy = vi.spyOn(api, "deleteNotification").mockResolvedValue({ ok: true });

    renderWithProviders(<InboxDrawer />);

    await screen.findByText("New sign-in from unusual IP");
    const deleteBtns = screen.getAllByRole("button", { name: /删除|Delete/i });
    fireEvent.click(deleteBtns[0]);

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledWith(1);
    });
  });

  it("filters unread notifications", async () => {
    const notifSpy = vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);

    renderWithProviders(<InboxDrawer />);

    const unreadTab = await screen.findByRole("button", { name: /未读|Unread/i });
    fireEvent.click(unreadTab);

    expect(useNotificationUIStore.getState().filter).toBe("unread");
    await waitFor(() => {
      expect(notifSpy).toHaveBeenCalledWith({ unreadOnly: true });
    });
  });

  it("renders gear link pointing to personal notification settings", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);

    renderWithProviders(<InboxDrawer />);

    const gearLink = await screen.findByTestId("inbox-preferences-link");
    expect(gearLink).toBeDefined();
    fireEvent.click(gearLink);
    expect(useNotificationUIStore.getState().isDrawerOpen).toBe(false);
  });
});

describe("InboxPage", () => {
  it("renders full inbox view with notifications and actions", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue(mockNotifications);

    renderWithProviders(<InboxPage />);

    expect(await screen.findByText("New sign-in from unusual IP")).toBeTruthy();
    expect(screen.getByText(/高优先级|High Priority/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /全部已读|Mark all/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /刷新|Refresh/i })).toBeTruthy();
  });

  it("renders empty state when no notifications match", async () => {
    vi.spyOn(api, "notifications").mockResolvedValue([]);

    renderWithProviders(<InboxPage />);

    expect(await screen.findByText(/收件箱为空|Inbox is empty/i)).toBeTruthy();
  });
});

describe("PreferencesPage", () => {
  it("renders event categories and dynamic channels in route matrix", async () => {
    vi.spyOn(api, "notificationPreferences").mockResolvedValue([
      { eventPattern: "system.*", channels: ["in_app", "email"] },
      { eventPattern: "security.*", channels: ["in_app", "email", "telegram"] },
    ]);
    vi.spyOn(api, "registeredNotificationChannels").mockResolvedValue(mockChannels);

    renderWithProviders(<PreferencesPage />);

    // The phone list and the desktop matrix both render, so each label appears
    // more than once in the DOM.
    expect((await screen.findAllByText("System events")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Security and audit").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Scheduled jobs").length).toBeGreaterThan(0);

    // Check dynamic channel columns
    expect(screen.getAllByText("In-App Inbox").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Email Service").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Telegram Bot").length).toBeGreaterThan(0);

    // Every channel switch names its row and its channel, for both layouts.
    const switches = screen.getAllByRole("switch");
    expect(switches.length).toBeGreaterThanOrEqual(5 * mockChannels.length);
    expect(screen.getAllByRole("switch", { name: "System events — In-App Inbox" }).length).toBe(2);
  });

  it("names the delete control of a custom rule and saves the matrix without it", async () => {
    vi.spyOn(api, "notificationPreferences").mockResolvedValue([
      { eventPattern: "billing.*", channels: ["in_app"] },
    ]);
    vi.spyOn(api, "registeredNotificationChannels").mockResolvedValue(mockChannels);
    const updateSpy = vi.spyOn(api, "updateNotificationPreferences").mockResolvedValue([]);

    renderWithProviders(<PreferencesPage />);

    const deleteButtons = await screen.findAllByRole("button", { name: "Delete — billing.*" });
    expect(deleteButtons.length).toBe(2);

    fireEvent.click(deleteButtons[0]);
    fireEvent.click(screen.getByRole("button", { name: /Save preferences/i }));

    await waitFor(() => expect(updateSpy).toHaveBeenCalled());
    const payload = updateSpy.mock.calls[0][0] as { eventPattern: string }[];
    expect(payload.some((row) => row.eventPattern === "billing.*")).toBe(false);
  });

  it("labels both fields of the add-rule dialog", async () => {
    vi.spyOn(api, "notificationPreferences").mockResolvedValue([]);
    vi.spyOn(api, "registeredNotificationChannels").mockResolvedValue(mockChannels);

    renderWithProviders(<PreferencesPage />);

    await screen.findAllByText("System events");
    fireEvent.click(screen.getByRole("button", { name: /Add rule/i }));

    expect(await screen.findByLabelText("Rule Name")).toBeTruthy();
    expect(screen.getByLabelText("Event Pattern Wildcard")).toBeTruthy();
  });

  it("saves updated preferences to API", async () => {
    vi.spyOn(api, "notificationPreferences").mockResolvedValue([]);
    vi.spyOn(api, "registeredNotificationChannels").mockResolvedValue(mockChannels);
    const updateSpy = vi.spyOn(api, "updateNotificationPreferences").mockResolvedValue([]);

    renderWithProviders(<PreferencesPage />);

    await screen.findAllByText("System events");
    const saveBtn = screen.getByRole("button", { name: /保存配置|Save preferences/i });

    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalled();
    });
  });

  it("resets preferences to defaults", async () => {
    vi.spyOn(api, "notificationPreferences").mockResolvedValue([
      { eventPattern: "system.*", channels: ["in_app"] },
    ]);
    vi.spyOn(api, "registeredNotificationChannels").mockResolvedValue(mockChannels);
    const resetSpy = vi.spyOn(api, "resetNotificationPreferences").mockResolvedValue({ ok: true });

    renderWithProviders(<PreferencesPage />);

    await screen.findAllByText("System events");
    const resetBtn = screen.getByRole("button", { name: /重置默认|Reset to default/i });

    fireEvent.click(resetBtn);

    await waitFor(() => {
      expect(resetSpy).toHaveBeenCalled();
    });
  });
});
