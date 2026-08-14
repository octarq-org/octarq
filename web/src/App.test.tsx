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
  it("redirects /license to /settings/license", async () => {
    Object.defineProperty(window, "location", {
      value: { pathname: "/license" },
      writable: true,
    });

    render(
      <MemoryRouter initialEntries={["/license"]}>
        <I18nProvider>
          <App />
        </I18nProvider>
      </MemoryRouter>
    );

    // If the redirect works, it hits /settings/license, which renders SettingsPage.
    // If it fails, it hits the catch-all "*", which renders PluginUnavailable ("Not part of this build").
    await waitFor(() => {
      expect(screen.queryByText(/Not part of this build/i)).toBeNull();
      expect(screen.getByTestId("settings-page")).toBeTruthy();
    }, { timeout: 2000 });
  });
});
