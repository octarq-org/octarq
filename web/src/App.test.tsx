// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
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
  it("sends /license to the instance console, not a tenant route", async () => {
    // The license is instance state, so its page moved to the /instance
    // console. That is a different router basename, so the tenant shell has to
    // leave with a full page load — a router <Navigate> would resolve the
    // target against /admin and land on the tenant catch-all.
    const replace = vi.fn();
    Object.defineProperty(window, "location", {
      value: { pathname: "/license", replace },
      writable: true,
    });

    render(
      <MemoryRouter initialEntries={["/license"]}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(replace).toHaveBeenCalledWith("/instance/license");
    }, { timeout: 2000 });
    // And it must not have quietly rendered the tenant catch-all instead.
    expect(screen.queryByText(/Not part of this build/i)).toBeNull();
  });
});
