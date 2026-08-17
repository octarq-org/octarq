// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../../i18n";
import { InstanceLinkSettings } from "./InstanceLinkSettings";

// The instance console page saves the deployment-wide reserved-slug list.
// Guard the wire: saving must PUT /api/instance-settings (the instance-level
// endpoint), never a tenant-scope path. fetch is stubbed at the global level —
// the suite's network guard (src/test/setup.ts) rejects real requests, so an
// un-mocked call would fail loudly instead of silently passing.
const INSTANCE_SETTINGS = {
  reservedSlugs: "pricing\nlogin",
  builtinReserved: ["admin", "api", "assets", "portal"],
  googleClientId: "",
  googleClientSecretSet: false,
  githubClientId: "",
  githubClientSecretSet: false,
  dataRetentionDays: 90,
  allowRegistration: true,
  requireEmailVerification: false,
  appName: "octarq",
  baseDomain: "",
  metricsTokenSet: false,
  ratelimitAuthRpm: 60,
  ratelimitApiRpm: 600,
  ratelimitRedirectRpm: 6000,
};

function jsonResponse(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    headers: { get: (name: string) => (name === "content-type" ? "application/json" : null) },
  };
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  cleanup();
  localStorage.clear();
  fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const method = (init?.method ?? "GET").toUpperCase();
    if (method === "GET" && input === "/api/instance-settings") return jsonResponse(INSTANCE_SETTINGS);
    if (method === "PUT" && input === "/api/instance-settings") {
      return jsonResponse({ ...INSTANCE_SETTINGS, reservedSlugs: "pricing\nlogin\nadminx" });
    }
    throw new Error(`unexpected request in test: ${method} ${input}`);
  });
  vi.stubGlobal("fetch", fetchMock);
});

// The reserved-slug list is newline-separated, so its textarea value is read
// via the raw .value rather than a display-value query (the library's default
// normalizer collapses whitespace and would never match an embedded newline).
async function findReservedSlugsTextarea(): Promise<HTMLTextAreaElement> {
  const textarea = (await screen.findByRole("textbox")) as HTMLTextAreaElement;
  await waitFor(() => expect(textarea.value).toBe("pricing\nlogin"));
  return textarea;
}

describe("InstanceLinkSettings", () => {
  it("loads the current reserved slugs from the instance-settings endpoint", async () => {
    render(
      <I18nProvider>
        <InstanceLinkSettings />
      </I18nProvider>,
    );

    await findReservedSlugsTextarea();

    const puts = (fetchMock.mock.calls as [RequestInfo | URL, RequestInit?][]).filter(
      ([, init]) => (init?.method ?? "GET").toUpperCase() === "PUT",
    );
    expect(puts).toHaveLength(0); // loading is a read, not a write
    expect(fetchMock).toHaveBeenCalledWith("/api/instance-settings", expect.objectContaining({ method: "GET" }));
  });

  it("saves reserved slugs by PUTting /api/instance-settings", async () => {
    render(
      <I18nProvider>
        <InstanceLinkSettings />
      </I18nProvider>,
    );

    const textarea = await findReservedSlugsTextarea();
    fireEvent.change(textarea, { target: { value: "pricing\nlogin\nadminx" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Settings" }));

    await waitFor(() => {
      const puts = (fetchMock.mock.calls as [RequestInfo | URL, RequestInit?][]).filter(
        ([, init]) => (init?.method ?? "GET").toUpperCase() === "PUT",
      );
      expect(puts).toHaveLength(1);
      // The save must hit the instance-level endpoint, not a tenant-scope one.
      expect(puts[0][0]).toBe("/api/instance-settings");
    });
  });
});