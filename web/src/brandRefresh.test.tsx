// @vitest-environment happy-dom
//
// Guards the per-workspace branding refresh.
//
// Branding is PER WORKSPACE, but brand.tsx caches it in module scope. That was
// safe only while switching workspaces did a full window.location.reload() — the
// page teardown was the cache invalidation. App.tsx replaced that with an in-app
// remount (faster, keeps module state alive), and nothing re-read the brand, so
// the first workspace's colours, name and logo stayed for the rest of the session.
//
// Three distinct failures are covered here, because they fail independently:
//   1. no re-read at all after a switch
//   2. a workspace with NO brand colour inheriting the previous one's accent
//      (an early return that never cleared the inline seed) — worse than stale,
//      this paints one tenant's colour onto another's workspace
//   3. the accent repainting (a DOM side effect) while the product name and logo
//      stayed behind, because a warm cache skipped the subscription
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import fs from "node:fs";
import path from "node:path";

type Cfg = { appName: string; logoUrl: string; brandColor: string; brandColor2: string };

const BASE = {
  googleEnabled: false,
  githubEnabled: false,
  registrationEnabled: true,
};

function cfg(over: Partial<Cfg>): Cfg & typeof BASE {
  return { ...BASE, appName: "octarq", logoUrl: "", brandColor: "", brandColor2: "", ...over };
}

// brand.tsx keeps module-level state, so each case needs a fresh module instance.
// The api spy has to be installed AFTER resetModules and on the freshly imported
// api object — spying the outer instance leaves the reloaded brand.tsx holding a
// different module, and the test makes real network calls instead.
async function freshBrand(...responses: Array<Cfg & typeof BASE>) {
  vi.resetModules();
  const { api } = await import("./api");
  const spy = vi.spyOn(api, "authConfig");
  for (const r of responses) spy.mockResolvedValueOnce(r);
  const brand = await import("./brand");
  return { ...brand, spy };
}

const seed = () => document.documentElement.style.getPropertyValue("--primary");

beforeEach(() => {
  document.documentElement.style.removeProperty("--primary");
  document.documentElement.style.removeProperty("--accent-violet");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("refreshBrand re-reads the brand for the new workspace", () => {
  it("applies the next workspace's accent", async () => {
    const { refreshBrand, spy } = await freshBrand(
      cfg({ brandColor: "#ea580c" }),
      cfg({ brandColor: "#0ea5e9" }),
    );

    await refreshBrand();
    expect(seed()).toBe("#ea580c");

    await refreshBrand();
    expect(seed()).toBe("#0ea5e9");
    expect(spy).toHaveBeenCalledTimes(2); // the cache must not swallow the second read
  });

  it("clears the seed for a workspace that sets no colour", async () => {
    const { refreshBrand } = await freshBrand(
      cfg({ brandColor: "#ea580c" }),
      cfg({ brandColor: "" }),
    );

    await refreshBrand();
    expect(seed()).toBe("#ea580c");

    // The unbranded workspace must fall back to the octarq default (no inline
    // seed at all), NOT keep the previous workspace's orange.
    await refreshBrand();
    expect(seed()).toBe("");
  });
});

describe("the shell re-renders on refresh, not just the CSS", () => {
  it("updates the product name after a refresh on a warm cache", async () => {
    const { BrandBridge, useAppName, refreshBrand } = await freshBrand(
      cfg({ appName: "Acme" }),
      cfg({ appName: "Globex" }),
    );
    function Name() {
      return <span data-testid="name">{useAppName()}</span>;
    }

    // Warm the cache BEFORE mounting: this is the workspace-switch shape, where
    // the shell is already up when the brand changes.
    await refreshBrand();
    render(
      <BrandBridge>
        <Name />
      </BrandBridge>,
    );
    await waitFor(() => expect(screen.getByTestId("name").textContent).toBe("Acme"));

    await refreshBrand();
    await waitFor(() => expect(screen.getByTestId("name").textContent).toBe("Globex"));
  });
});

describe("the shell wires the refresh to the workspace switch", () => {
  // The behaviour above is worthless if nothing calls it. switchToOrg is plain
  // shell state — remounting the whole app to assert this would test routing,
  // not the wiring — so read the call site.
  const app = fs.readFileSync(path.resolve(__dirname, "App.tsx"), "utf8");

  it("switchToOrg calls refreshBrand", () => {
    const start = app.indexOf("function switchToOrg");
    expect(start, "switchToOrg not found in App.tsx").toBeGreaterThan(-1);
    const body = app.slice(start, app.indexOf("\n  }", start));
    expect(
      body,
      "switchToOrg no longer refreshes the brand; a workspace switch will keep the previous workspace's colours",
    ).toContain("refreshBrand()");
  });
});
