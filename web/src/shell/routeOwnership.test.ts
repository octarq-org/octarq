// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";
import { registerUIPlugin, resetRegistry, type UIPlugin } from "@octarq/plugin-sdk";
import { dashboardOwnsPath } from "./routeOwnership";

function pluginWith(path: string): UIPlugin {
  return { name: "fake-plugin", routes: [{ path, Component: () => null }] } as unknown as UIPlugin;
}

afterEach(() => resetRegistry());

describe("dashboardOwnsPath", () => {
  it("owns the static routes of the shell", () => {
    expect(dashboardOwnsPath("/")).toBe(true);
    expect(dashboardOwnsPath("/overview")).toBe(true);
    expect(dashboardOwnsPath("/settings/general")).toBe(true);
    expect(dashboardOwnsPath("/settings/gateway")).toBe(true);
  });

  it("does not own a page served by another application", () => {
    expect(dashboardOwnsPath("/aiduu/")).toBe(false);
    expect(dashboardOwnsPath("/aiduu/?room=3")).toBe(false);
    expect(dashboardOwnsPath("/status")).toBe(false);
    expect(dashboardOwnsPath("/instance/settings")).toBe(false);
  });

  it("owns the prefixes of composed frontend plugins", () => {
    expect(dashboardOwnsPath("/domains")).toBe(false);
    registerUIPlugin(pluginWith("/domains"));
    expect(dashboardOwnsPath("/domains")).toBe(true);
    expect(dashboardOwnsPath("/domains?tab=providers")).toBe(true);
    resetRegistry();
    expect(dashboardOwnsPath("/domains")).toBe(false);
  });
});
