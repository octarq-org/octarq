// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App";
import { I18nProvider } from "./i18n";
import { api } from "./api";

// Mock API responses so the app boots successfully.
vi.mock("./api", () => {
  return {
    api: {
      authConfig: vi.fn().mockResolvedValue({
        googleEnabled: false,
        githubEnabled: false,
        registrationEnabled: true,
        appName: "Octarq",
      }),
      login: vi.fn().mockResolvedValue({ ok: true, email: "test@example.com" }),
      me: vi.fn().mockResolvedValue({
        email: "test@example.com",
        orgId: 1,
        orgName: "Test Org",
      }),
      getOrgs: vi.fn().mockResolvedValue([]),
      getUserSettings: vi.fn().mockResolvedValue({}),
      orgs: vi.fn().mockResolvedValue([]),
      settings: vi.fn().mockResolvedValue({ isInstanceAdmin: true }),
      menus: vi.fn().mockResolvedValue([]),
      plugins: vi.fn().mockResolvedValue([]),
      actions: vi.fn().mockResolvedValue([]),
      helpIndex: vi.fn().mockResolvedValue([]),
      helpCategories: vi.fn().mockResolvedValue([]),
      instanceBuild: vi.fn().mockResolvedValue({ version: "dev", commit: "unknown", builtAt: "unknown" }),
    },
  };
});

// Mock lazy-loaded pages to make assertions easy and prevent cascading network/render issues.
vi.mock("./pages/Settings", () => ({
  default: () => <div data-testid="settings-page">Settings Page</div>,
}));

vi.mock("./pages/Overview", () => ({
  default: () => <div data-testid="overview-page">Overview Page</div>,
}));

vi.mock("./pages/OverviewPage", () => ({
  default: () => <div data-testid="overview-page">Overview Page</div>,
}));

vi.mock("./plugins/PluginRoutes", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./plugins/PluginRoutes")>();
  return {
    ...actual,
    pluginRouteElements: () => [],
  };
});

describe("App Routing", () => {
  it("sends /link-settings to the instance console, not a tenant route", async () => {
    // Instance-level settings live in the /instance console. That is a different
    // router basename, so the tenant shell has to leave with a full page load —
    // a router <Navigate> would resolve the target against /admin and land on
    // the tenant catch-all.
    const replace = vi.fn();
    Object.defineProperty(window, "location", {
      value: { pathname: "/link-settings", replace },
      writable: true,
    });

    render(
      <MemoryRouter initialEntries={["/link-settings"]}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(replace).toHaveBeenCalledWith("/instance/settings");
    }, { timeout: 2000 });
    // And it must not have quietly rendered the tenant catch-all instead.
    expect(screen.queryByText(/Not part of this build/i)).toBeNull();
  });

  it("navigates to next destination upon successful login when ?next= is specified", async () => {
    vi.mocked(api.me).mockRejectedValue(new Error("401 Unauthorized"));

    Object.defineProperty(window, "location", {
      value: { pathname: "/admin/", search: "?next=%2Fsettings" },
      writable: true,
    });

    render(
      <MemoryRouter initialEntries={["/?next=%2Fsettings"]}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </MemoryRouter>
    );

    const emailInput = await screen.findByLabelText(/email/i);
    const passInput = screen.getByLabelText(/password/i);

    vi.mocked(api.login).mockResolvedValueOnce({ ok: true, email: "user@example.com" });
    vi.mocked(api.me).mockResolvedValue({ email: "user@example.com", orgId: 1 });

    fireEvent.change(emailInput, { target: { value: "user@example.com" } });
    fireEvent.change(passInput, { target: { value: "secret123" } });
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));

    await waitFor(() => {
      expect(screen.getByTestId("settings-page")).toBeTruthy();
    });
  });

  it("navigates to /overview upon successful login when no next param is specified", async () => {
    vi.mocked(api.me).mockRejectedValue(new Error("401 Unauthorized"));

    Object.defineProperty(window, "location", {
      value: { pathname: "/admin/", search: "" },
      writable: true,
    });

    render(
      <MemoryRouter initialEntries={["/"]}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </MemoryRouter>
    );

    const emailInput = await screen.findByLabelText(/email/i);
    const passInput = screen.getByLabelText(/password/i);

    vi.mocked(api.login).mockResolvedValueOnce({ ok: true, email: "user@example.com" });
    vi.mocked(api.me).mockResolvedValue({ email: "user@example.com", orgId: 1 });

    fireEvent.change(emailInput, { target: { value: "user@example.com" } });
    fireEvent.change(passInput, { target: { value: "secret123" } });
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));

    await waitFor(() => {
      expect(screen.getByTestId("overview-page")).toBeTruthy();
    });
  });
});
