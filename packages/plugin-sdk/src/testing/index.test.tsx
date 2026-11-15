// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TestHarness, mockPluginApi } from "./index";
import { usePluginData } from "../data/hooks";
import { useTranslation } from "../i18n";
import { useAppName } from "../brand";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Page() {
  const { t } = useTranslation();
  const app = useAppName();
  const { data } = usePluginData<{ count: number }>(["probe"], "/api/x/probe");
  return (
    <div>
      <span data-testid="title">{t("probe.title", "Fallback title")}</span>
      <span data-testid="brand">{app}</span>
      <span data-testid="count">{data ? data.count : "pending"}</span>
    </div>
  );
}

describe("TestHarness", () => {
  it("provides i18n, brand and a working query client", async () => {
    const api = mockPluginApi([{ path: "/api/x/probe", body: { count: 4 } }]);
    try {
      render(
        <TestHarness resources={{ en: { probe: { title: "Harness title" } } }} brand="acme">
          <Page />
        </TestHarness>,
      );
      expect((await screen.findByTestId("title")).textContent).toBe("Harness title");
      expect(screen.getByTestId("brand").textContent).toBe("acme");
      expect(await screen.findByText("4")).toBeTruthy();
      expect(api.calls).toHaveLength(1);
    } finally {
      api.restore();
    }
  });

  it("hides a plugin when the gate disables it", async () => {
    const api = mockPluginApi([{ path: "/api/x/probe", body: { count: 1 } }]);
    try {
      render(
        <TestHarness gate={{ disabledPlugins: new Set(["dns"]) }}>
          <div>plain</div>
        </TestHarness>,
      );
      expect(await screen.findByText("plain")).toBeTruthy();
    } finally {
      api.restore();
    }
  });
});

describe("mockPluginApi", () => {
  it("answers an undeclared route with 404 so a typo fails loudly", async () => {
    const api = mockPluginApi([]);
    try {
      const res = await fetch("/api/x/missing");
      expect(res.status).toBe(404);
      expect(api.calls).toEqual([{ method: "GET", url: "/api/x/missing", body: undefined }]);
    } finally {
      api.restore();
    }
  });

  it("records the parsed body and method, and matches by pattern", async () => {
    const api = mockPluginApi([{ method: "POST", path: /^\/api\/x\/things\/\d+$/, body: { ok: true } }]);
    try {
      const res = await fetch("/api/x/things/12", {
        method: "POST",
        body: JSON.stringify({ name: "a" }),
      });
      expect(res.status).toBe(200);
      expect(await res.json()).toEqual({ ok: true });
      expect(api.calls[0]).toMatchObject({ method: "POST", url: "/api/x/things/12", body: { name: "a" } });
    } finally {
      api.restore();
    }
  });

  it("restores the previous fetch", () => {
    const before = globalThis.fetch;
    const api = mockPluginApi([]);
    expect(globalThis.fetch).not.toBe(before);
    api.restore();
    expect(globalThis.fetch).toBe(before);
  });
});
