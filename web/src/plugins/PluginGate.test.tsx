// @vitest-environment happy-dom
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import { PluginGate, PluginGateContext, usePluginGate } from "./PluginGate";
import { I18nProvider } from "../i18n";
import { MemoryRouter } from "react-router-dom";
import type { UIPlugin, UIRoute } from "@octarq/plugin-sdk";
import { useEffect, lazy } from "react";

function Trigger404() {
  const gate = usePluginGate();
  useEffect(() => {
    gate.degrade(404);
  }, [gate]);
  return null;
}

const dummyPlugin: UIPlugin = {
  name: "test-plugin",
  routes: [],
  widgets: [],
};

const dummyRoute: UIRoute = {
  path: "/test",
  Component: lazy(async () => ({ default: () => <div>Test Route</div> })),
};

afterEach(() => {
  cleanup();
});

describe("PluginGate", () => {
  it("(a) renders PluginDisabled when disabledPlugins matches", () => {
    const disabledPlugins = new Set<string>(["test-plugin"]);
    const disabledPaths = new Set<string>();

    render(
      <MemoryRouter>
        <I18nProvider>
          <PluginGateContext.Provider value={{ loaded: true, disabledPlugins, disabledPaths } as any}>
            <PluginGate plugin={dummyPlugin} route={dummyRoute}>
              <div>Content</div>
            </PluginGate>
          </PluginGateContext.Provider>
        </I18nProvider>
      </MemoryRouter>
    );

    expect(screen.getByText("Plugin disabled")).not.toBeNull();
    expect(screen.queryByText("Not part of this build")).toBeNull();
  });

  it("(b) renders PluginUnavailable for 404 degrade", async () => {
    const disabledPlugins = new Set<string>();
    const disabledPaths = new Set<string>();

    render(
      <MemoryRouter>
        <I18nProvider>
          <PluginGateContext.Provider value={{ loaded: true, disabledPlugins, disabledPaths } as any}>
            <PluginGate plugin={dummyPlugin} route={dummyRoute}>
              <Trigger404 />
            </PluginGate>
          </PluginGateContext.Provider>
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Not part of this build")).not.toBeNull();
    });
    expect(screen.queryByText("Plugin disabled")).toBeNull();
  });

  // The plugin list arrives from /api/plugins, so a page can 404 before it has
  // loaded. Until then the pre-check in PluginGate cannot fire and the answer
  // has to come from the degrade path instead.
  it("(c) renders PluginDisabled for a 404 degrade on a disabled plugin", async () => {
    const disabledPlugins = new Set<string>(["test-plugin"]);
    const disabledPaths = new Set<string>();

    render(
      <MemoryRouter>
        <I18nProvider>
          <PluginGateContext.Provider value={{ loaded: false, disabledPlugins, disabledPaths } as any}>
            <PluginGate plugin={dummyPlugin} route={dummyRoute}>
              <Trigger404 />
            </PluginGate>
          </PluginGateContext.Provider>
        </I18nProvider>
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByText("Plugin disabled")).not.toBeNull();
    });
    expect(screen.queryByText("Not part of this build")).toBeNull();
  });
});
