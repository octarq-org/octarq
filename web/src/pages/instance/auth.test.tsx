// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api, InstanceSettings } from "../../api";
import { I18nProvider } from "../../i18n";
import * as ui from "../../ui";
import { AuthenticationSettings } from "./auth";

function makeSettings(overrides: Partial<InstanceSettings> = {}): InstanceSettings {
  return {
    reservedSlugs: "",
    builtinReserved: [],
    googleClientId: "",
    googleClientSecretSet: false,
    githubClientId: "",
    githubClientSecretSet: false,
    dataRetentionDays: 90,
    allowRegistration: true,
    requireEmailVerification: false,
    appName: "Octarq",
    baseDomain: "",
    metricsTokenSet: false,
    ratelimitAuthRpm: 60,
    ratelimitApiRpm: 600,
    ratelimitRedirectRpm: 6000,
    ...overrides,
  };
}

function renderAuthSettings() {
  return render(
    <I18nProvider>
      <AuthenticationSettings />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("AuthenticationSettings suite", () => {
  it("renders authentication settings with toggles and provider accordions", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(
      makeSettings({
        allowRegistration: true,
        requireEmailVerification: false,
        googleClientId: "google-client-id-123",
        googleClientSecretSet: true,
      }),
    );

    renderAuthSettings();

    expect(await screen.findByText(/allow public sign-up/i)).toBeTruthy();
    expect(screen.getByText(/require email verification/i)).toBeTruthy();
    expect(screen.getByText(/email registration & password sign-in/i)).toBeTruthy();
    expect(screen.getByText("Google")).toBeTruthy();
    expect(screen.getByText("GitHub")).toBeTruthy();
  });

  it("toggles public sign-up setting", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings({ allowRegistration: true }));
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(makeSettings({ allowRegistration: false }));
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderAuthSettings();

    const switches = await screen.findAllByRole("switch");
    fireEvent.click(switches[0]);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({ allowRegistration: false });
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("toggles require email verification setting", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings({ requireEmailVerification: false }));
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(makeSettings({ requireEmailVerification: true }));
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderAuthSettings();

    const switches = await screen.findAllByRole("switch");
    fireEvent.click(switches[1]);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({ requireEmailVerification: true });
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("configures and saves Google OAuth provider", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(makeSettings({ googleClientId: "" }));
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(
      makeSettings({ googleClientId: "new-google-id", googleClientSecretSet: true }),
    );
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderAuthSettings();

    const googleAccordionBtn = await screen.findByRole("button", { name: /google/i });
    fireEvent.click(googleAccordionBtn);

    const googleIdInput = screen.getByPlaceholderText("*.apps.googleusercontent.com");
    fireEvent.change(googleIdInput, { target: { value: "new-google-id" } });

    const googleSecretInput = screen.getByPlaceholderText("Secret value");
    fireEvent.change(googleSecretInput, { target: { value: "google-secret-value" } });

    const saveButtons = screen.getAllByRole("button", { name: /^save$/i });
    fireEvent.click(saveButtons[0]);

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({
        googleClientId: "new-google-id",
        googleClientSecret: "google-secret-value",
      });
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("clears Google client secret after confirmation", async () => {
    vi.spyOn(api, "instanceSettings").mockResolvedValue(
      makeSettings({ googleClientId: "google-id", googleClientSecretSet: true }),
    );
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const updateSpy = vi.spyOn(api, "updateInstanceSettings").mockResolvedValue(
      makeSettings({ googleClientId: "google-id", googleClientSecretSet: false }),
    );
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderAuthSettings();

    const googleAccordionBtn = await screen.findByRole("button", { name: /google/i });
    fireEvent.click(googleAccordionBtn);

    const clearBtn = screen.getByRole("button", { name: /clear/i });
    fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(updateSpy).toHaveBeenCalledWith({ googleClientSecret: "" });
      expect(toastSpy).toHaveBeenCalled();
    });
  });
});
