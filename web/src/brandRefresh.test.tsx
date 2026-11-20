// @vitest-environment happy-dom
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

beforeAll(async () => {
  await import("./api");
  await import("./brand");
}, 60_000);

const seed = () => document.documentElement.style.getPropertyValue("--primary");

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
    expect(spy).toHaveBeenCalledTimes(2);
  });

  it("clears the seed for a workspace that sets no colour", async () => {
    const { refreshBrand } = await freshBrand(
      cfg({ brandColor: "#ea580c" }),
      cfg({ brandColor: "" }),
    );

    await refreshBrand();
    expect(seed()).toBe("#ea580c");

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
    expect(href).toContain("%23ea580c");
    expect(href).not.toContain("#ea580c");
    expect(icon?.getAttribute("type")).toBe("image/svg+xml");
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe(
      "#ea580c",
    );
  });

  it("restores the default favicon and removes theme-color for a colourless workspace", async () => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    const defaultValue = link!.href;

    const { refreshBrand } = await freshBrand(
      cfg({ brandColor: "#ea580c" }),
      cfg({ brandColor: "" }),
    );

    await refreshBrand();
    expect(document.querySelector('link[rel="icon"]')?.getAttribute("href")).toContain(
      "%23ea580c",
    );

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

    spy.mockRejectedValueOnce(new Error("network down"));
    await refreshBrand();
    expect(link?.href).toBe(defaultValue);
    expect(document.querySelector('meta[name="theme-color"]')).toBeNull();
  });
});

describe("the shell wires the refresh to the workspace switch", () => {
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
