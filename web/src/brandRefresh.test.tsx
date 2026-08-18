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
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
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
//
// Responses come from an explicit queue rather than stacked mockResolvedValueOnce
// entries: once that queue is empty, mockResolvedValueOnce silently falls back to
// the REAL authConfig — a live fetch to /api/auth/config — and the failure turns
// machine-dependent (a local proxy hang, a CI refusal) instead of a test failure.
// The queue throws synchronously instead, which also matters because brand.tsx's
// load() swallows async rejections via `.catch()`: only a synchronous throw
// surfaces through refreshBrand() to name the missing response.
async function freshBrand(...responses: Array<Cfg & typeof BASE>) {
  vi.resetModules();
  const { api } = await import("./api");
  const queue = [...responses];
  const spy = vi.spyOn(api, "authConfig").mockImplementation(() => {
    const r = queue.shift();
    if (r === undefined) {
      throw new Error(
        `authConfig called more times than the test queued responses (${responses.length}) — ` +
          "a real request to GET /api/auth/config was blocked instead",
      );
    }
    return Promise.resolve(r);
  });
  const brand = await import("./brand");
  return { ...brand, spy };
}

// Warm the module graph once, outside any test's clock. freshBrand() does a
// vi.resetModules() + dynamic import of brand.tsx, which pulls React, the plugin
// SDK and api.ts through the transform pipeline the first time it runs — on a
// loaded machine that cold start alone measured 9s and blew the 5s default
// timeout, failing whichever case happened to be first. Re-instantiating the
// modules afterwards is cheap; only the transform is not, and it is cached
// across resetModules. The hook gets its own generous budget because it is the
// one place that pays the cost.
beforeAll(async () => {
  await import("./api");
  await import("./brand");
}, 60_000);

const seed = () => document.documentElement.style.getPropertyValue("--primary");

// Seed the document the way index.html ships it: a favicon link (the Octarq
// mark) and no theme-color meta. Each test gets a fresh icon link so
// defaultFavicon's one-time capture targets the markup default, and any
// theme-color meta left by a previous case is cleared.
beforeEach(() => {
  document.documentElement.style.removeProperty("--primary");
  document.documentElement.style.removeProperty("--accent-violet");
  document.querySelector('meta[name="theme-color"]')?.remove();
  document.querySelector('link[rel="icon"]')?.remove();
  document.head.insertAdjacentHTML(
    "beforeend",
    '<link rel="icon" href="/favicon.svg" type="image/svg+xml" />',
  );
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

describe("the tab icon and theme-color follow the brand colour", () => {
  it("brands the favicon with the colour when the workspace sets only a colour", async () => {
    const { refreshBrand } = await freshBrand(cfg({ brandColor: "#ea580c" }));

    await refreshBrand();

    const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    const href = icon?.getAttribute("href") ?? "";
    expect(href.startsWith("data:image/svg+xml,")).toBe(true);
    // The '#' must be percent-encoded inside the data URI — a raw '#' would
    // read as a URL fragment separator and the icon would never display.
    expect(href).toContain("%23ea580c");
    expect(href).not.toContain("#ea580c");
    expect(icon?.getAttribute("type")).toBe("image/svg+xml");
    // The browser chrome picks up the same colour the CSS variables got.
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      "#ea580c",
    );
  });

  it("restores the default favicon and removes theme-color for a colourless workspace", async () => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    const defaultValue = link!.href; // captured the same way the module captures it

    const { refreshBrand } = await freshBrand(
      cfg({ brandColor: "#ea580c" }),
      cfg({ brandColor: "" }),
    );

    await refreshBrand();
    expect(document.querySelector('link[rel="icon"]')?.getAttribute("href")).toContain(
      "%23ea580c",
    );

    // The unbranded workspace must fall back to the markup default — not keep
    // the previous workspace's branded icon — and the theme-color meta must be
    // REMOVED, not left behind with an empty or stale value.
    await refreshBrand();
    expect(link?.href).toBe(defaultValue);
    expect(link?.getAttribute("href")).not.toContain("data:image/svg+xml");
    expect(link?.getAttribute("type")).toBe("image/svg+xml");
    expect(document.querySelector('meta[name="theme-color"]')).toBeNull();
  });

  it("lets the white-label logo win over the brand colour", async () => {
    const { refreshBrand } = await freshBrand(
      cfg({ logoUrl: "https://cdn.example/logo.png", brandColor: "#ea580c" }),
    );

    await refreshBrand();

    const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    expect(icon?.getAttribute("href")).toBe("https://cdn.example/logo.png");
    expect(icon?.getAttribute("href")).not.toContain("data:image/svg+xml");
  });

  it("drops to the colourless default when the brand fetch fails", async () => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    const defaultValue = link!.href;

    const { refreshBrand, spy } = await freshBrand(cfg({ brandColor: "#ea580c" }));
    await refreshBrand();
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      "#ea580c",
    );

    // A failed fetch leaves no branding to trust: the previous workspace's
    // colour must not linger in the tab (the failure class the file header
    // documents as #2).
    spy.mockRejectedValueOnce(new Error("network down"));
    await refreshBrand();
    expect(link?.href).toBe(defaultValue);
    expect(document.querySelector('meta[name="theme-color"]')).toBeNull();
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
