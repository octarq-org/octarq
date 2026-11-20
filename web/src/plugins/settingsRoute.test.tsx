// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import { lazy } from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { pluginRouteElements } from "./PluginRoutes";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { PluginGateContext } from "./PluginGate";
import { I18nProvider } from "../i18n";

function DummySettingsPage() {
  return <div data-testid="settings-page">Settings Page Catch-All</div>;
}

function DummyPluginPage() {
  return <div data-testid="plugin-page">Plugin Settings Page</div>;
}

const dummyPlugin: UIPlugin = {
  name: "demo-plugin",
  routes: [
    {
      path: "/settings/demo",
      Component: lazy(async () => ({ default: DummyPluginPage })),
    },
  ],
  widgets: [],
};

vi.mock("@octarq/plugin-sdk", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@octarq/plugin-sdk")>();
  return {
    ...actual,
    uiPlugins: () => [dummyPlugin],
  };
});

describe("Settings Route Namespace", () => {
  it("routes /settings/demo to the plugin instead of the /settings/* splat", async () => {
    const disabledPlugins = new Set<string>();
    const disabledPaths = new Set<string>();

    render(
      <MemoryRouter initialEntries={["/settings/demo"]}>
        <I18nProvider>
          <PluginGateContext.Provider value={{ loaded: true, disabledPlugins, disabledPaths } as any}>
            <Routes>
              <Route path="/settings/*" element={<DummySettingsPage />} />
              {pluginRouteElements()}
            </Routes>
          </PluginGateContext.Provider>
        </I18nProvider>
      </MemoryRouter>
    );

    expect(await screen.findByTestId("plugin-page")).not.toBeNull();
    expect(screen.queryByTestId("settings-page")).toBeNull();
  });
});
