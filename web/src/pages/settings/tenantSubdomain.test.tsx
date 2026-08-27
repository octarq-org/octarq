// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../i18n";
import { GeneralSettings } from "./general";
import { InstanceSettings } from "../instance/settings";

vi.mock("../../api", () => {
  const api = {
    settings: vi.fn(async () => ({
      reservedMailboxes: "",
      orgSlug: "acme",
      tenantSubdomain: "acme.app.example.com",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: true,
    })),
    instanceSettings: vi.fn(async () => ({
      reservedSlugs: "",
      builtinReserved: [],
      googleClientId: "",
      googleClientSecretSet: false,
      githubClientId: "",
      githubClientSecretSet: false,
      dataRetentionDays: 90,
      allowRegistration: true,
      requireEmailVerification: true,
      appName: "",
      baseDomain: "app.example.com",
      metricsTokenSet: false,
      ratelimitAuthRpm: 60,
      ratelimitApiRpm: 600,
      ratelimitRedirectRpm: 6000,
    })),
    updateInstanceSettings: vi.fn(async (s: any) => s),
    downloadBackup: vi.fn(async () => ({ blob: new Blob(), filename: "backup" })),
    me: vi.fn(async () => ({ orgId: 1 })),
    orgs: vi.fn(async () => [{ id: 1, name: "Acme", role: "owner" }]),
    orgSlug: vi.fn(async () => ({ slug: "acme" })),
    updateOrgSlug: vi.fn(async () => ({ slug: "acme" })),
    updateOrg: vi.fn(async () => ({ id: 1 })),
    exportWorkspaceData: vi.fn(async () => ({})),
    purgeWorkspaceData: vi.fn(async () => ({ ok: true })),
    instanceBuild: vi.fn(async () => ({ version: "dev", commit: "unknown", builtAt: "unknown" })),
    smtpSenders: vi.fn(async () => []),
  };
  return { api };
});

describe("Tenant subdomain settings UI", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the workspace's tenant address as copyable code", async () => {
    render(
      <I18nProvider>
        <GeneralSettings />
      </I18nProvider>,
    );
    await waitFor(() => {
      expect(screen.getByText("Tenant address")).not.toBeNull();
    });
    // The address itself renders inside the click-to-copy Code element.
    expect(await screen.findByText("acme.app.example.com")).not.toBeNull();
    const code = screen.getByText("acme.app.example.com").closest("code");
    expect(code).not.toBeNull();
    expect(code?.getAttribute("role")).toBe("button");
  });

  it("renders the instance-level base domain field pre-filled", async () => {
    render(
      <I18nProvider>
        <InstanceSettings />
      </I18nProvider>,
    );
    await waitFor(() => {
      expect(screen.getByDisplayValue("app.example.com")).not.toBeNull();
    });
  });
});
