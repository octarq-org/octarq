// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api, InstanceSettings as IInstanceSettings, SMTPSender } from "../../api";
import { I18nProvider } from "../../i18n";
import * as ui from "../../ui";
import { InstanceSettings } from "./settings";

function makeSettings(overrides: Partial<IInstanceSettings> = {}): IInstanceSettings {
  return {
    reservedSlugs: "admin,api,login",
    builtinReserved: ["admin", "api"],
    googleClientId: "",
    googleClientSecretSet: false,
    githubClientId: "",
    githubClientSecretSet: false,
    dataRetentionDays: 90,
    allowRegistration: true,
    requireEmailVerification: false,
    appName: "Octarq Core",
    baseDomain: "octarq.io",
    sharedHosts: "go.octarq.io",
    metricsTokenSet: false,
    ratelimitAuthRpm: 60,
    ratelimitApiRpm: 600,
    ratelimitRedirectRpm: 6000,
    publicCorsOrigins: "https://example.com",
    systemSenderId: 0,
    ...overrides,
  };
}

function renderInstanceSettings() {
  return render(
    <I18nProvider>
      <InstanceSettings />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("InstanceSettings suite", () => {
  it("loads and renders general instance configuration, senders, and build info", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings());
    vi.spyOn(api, "smtpSenders").mockResolvedValue([
      { id: 1, name: "Resend", host: "smtp.resend.com", port: 587, user: "resend", fromEmail: "noreply@octarq.io", passSet: true, createdAt: "2026-01-01" },
    ]);
    vi.spyOn(api, "instanceBuild").mockResolvedValue({
      version: "v1.2.3",
      commit: "abcdef123456",
      builtAt: "2026-08-30T12:00:00Z",
    });

    renderInstanceSettings();

    expect(await screen.findByDisplayValue("Octarq Core")).toBeTruthy();
    expect(screen.getByDisplayValue("octarq.io")).toBeTruthy();
    expect(screen.getByDisplayValue("go.octarq.io")).toBeTruthy();
    expect(screen.getByDisplayValue("https://example.com")).toBeTruthy();
    expect(screen.getByDisplayValue("admin,api,login")).toBeTruthy();
    expect(screen.getByText("v1.2.3")).toBeTruthy();
    expect(screen.getByText("abcdef12")).toBeTruthy();
  });

  it("updates general instance settings and saves payload", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings());
    vi.spyOn(api, "smtpSenders").mockResolvedValue([]);
    vi.spyOn(api, "instanceBuild").mockResolvedValue({ version: "dev", commit: "", builtAt: "" });
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(
      makeSettings({ appName: "Octarq Pro", baseDomain: "app.octarq.io" }),
    );
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderInstanceSettings();

    const appNameInput = await screen.findByDisplayValue("Octarq Core");
    fireEvent.change(appNameInput, { target: { value: "Octarq Pro" } });

    const baseDomainInput = screen.getByDisplayValue("octarq.io");
    fireEvent.change(baseDomainInput, { target: { value: "app.octarq.io" } });

    const saveButtons = screen.getAllByRole("button", { name: /^save$/i });
    fireEvent.click(saveButtons[0]);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          appName: "Octarq Pro",
          baseDomain: "app.octarq.io",
        }),
      );
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("clears metrics token after confirmation dialog", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings({ metricsTokenSet: true }));
    vi.spyOn(api, "smtpSenders").mockResolvedValue([]);
    vi.spyOn(api, "instanceBuild").mockResolvedValue({ version: "dev", commit: "", builtAt: "" });
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(
      makeSettings({ metricsTokenSet: false }),
    );
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderInstanceSettings();

    const clearBtn = await screen.findByRole("button", { name: /clear/i });
    fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(updateSpy).toHaveBeenCalledWith({ metricsToken: "" });
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("triggers database backup download", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings());
    vi.spyOn(api, "smtpSenders").mockResolvedValue([]);
    vi.spyOn(api, "instanceBuild").mockResolvedValue({ version: "dev", commit: "", builtAt: "" });
    const backupSpy = vi.spyOn(api, "downloadBackup").mockResolvedValue({
      blob: new Blob(["backup content"], { type: "application/octet-stream" }),
      filename: "octarq-backup-2026.db",
    });

    renderInstanceSettings();

    const backupBtn = await screen.findByRole("button", { name: /download backup/i });
    fireEvent.click(backupBtn);

    await waitFor(() => {
      expect(backupSpy).toHaveBeenCalled();
    });
  });

  it("updates rate limiting configuration", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings());
    vi.spyOn(api, "smtpSenders").mockResolvedValue([]);
    vi.spyOn(api, "instanceBuild").mockResolvedValue({ version: "dev", commit: "", builtAt: "" });
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(
      makeSettings({ ratelimitAuthRpm: 120, ratelimitApiRpm: 1200 }),
    );
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderInstanceSettings();

    const authRpmInput = await screen.findByDisplayValue("60");
    fireEvent.change(authRpmInput, { target: { value: "120" } });

    const saveButtons = screen.getAllByRole("button", { name: /^save$/i });
    fireEvent.click(saveButtons[1]); // Rate limiting section save button

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          ratelimitAuthRpm: 120,
        }),
      );
      expect(toastSpy).toHaveBeenCalled();
    });
  });
});
