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
  // A settings page owned by a plugin has to win over the shell's /settings/*
  // catch-all, or every plugin in the Settings area renders the settings shell's
  // fallback instead of itself. Nothing arranges that explicitly — it falls out
  // of react-router ranking static segments above splats — so it needs pinning:
  // moving SettingsPage into a nested <Routes>, or splitting the plugin routes
  // into a second <Routes>, silently takes it away.
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

    // findBy*, not getBy*: the plugin page is lazy(), so it is absent on the
    // first paint whether or not the route matched. A synchronous assertion here
    // fails either way and pins nothing.
    expect(await screen.findByTestId("plugin-page")).not.toBeNull();
    expect(screen.queryByTestId("settings-page")).toBeNull();
  });
});
