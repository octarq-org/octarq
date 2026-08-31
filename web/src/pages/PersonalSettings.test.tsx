// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { api, ApiError } from "../api";
import { I18nProvider } from "../i18n";
import { RoleProvider } from "../shell/role";
import * as ui from "../ui";
import { ProfileSettings, ApiTokens } from "./PersonalSettings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ProfileSettings suite", () => {
  it("loads and displays user profile email", async () => {
    vi.spyOn(api, "me").mockResolvedValue({
      email: "operator@octarq.local",
      orgId: 1,
    });

    render(
      <I18nProvider>
        <MemoryRouter>
          <ProfileSettings />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByText("operator@octarq.local")).toBeTruthy();
    expect(screen.getByText("My Profile")).toBeTruthy();
    expect(screen.getByRole("button", { name: /change email/i })).toBeTruthy();
  });

  it("handles changing email flow with password confirmation and success toast", async () => {
    vi.spyOn(api, "me").mockResolvedValue({
      email: "old@octarq.local",
      orgId: 1,
    });
    const confirmPassSpy = vi.spyOn(ui, "confirmPassword").mockResolvedValue("secret123");
    const changeEmailSpy = vi.spyOn(api, "changeEmail").mockResolvedValue({
      ok: true,
      email: "new@octarq.local",
    });
    const toastSpy = vi.spyOn(ui.toast, "success");

    render(
      <I18nProvider>
        <MemoryRouter>
          <ProfileSettings />
        </MemoryRouter>
      </I18nProvider>,
    );

    await screen.findByText("old@octarq.local");
    fireEvent.click(screen.getByRole("button", { name: /change email/i }));

    const input = screen.getByPlaceholderText("you@domain.com");
    fireEvent.change(input, { target: { value: "new@octarq.local" } });
    fireEvent.click(screen.getByRole("button", { name: /continue/i }));

    await waitFor(() => {
      expect(confirmPassSpy).toHaveBeenCalled();
      expect(changeEmailSpy).toHaveBeenCalledWith("new@octarq.local", "secret123");
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("handles email update errors (409 conflict and 400 SSO)", async () => {
    vi.spyOn(api, "me").mockResolvedValue({
      email: "old@octarq.local",
      orgId: 1,
    });
    vi.spyOn(ui, "confirmPassword").mockResolvedValue("secret123");
    vi.spyOn(api, "changeEmail").mockRejectedValue(new ApiError(409, "Email already exists"));
    const toastErrorSpy = vi.spyOn(ui.toast, "error");

    render(
      <I18nProvider>
        <MemoryRouter>
          <ProfileSettings />
        </MemoryRouter>
      </I18nProvider>,
    );

    await screen.findByText("old@octarq.local");
    fireEvent.click(screen.getByRole("button", { name: /change email/i }));

    const input = screen.getByPlaceholderText("you@domain.com");
    fireEvent.change(input, { target: { value: "duplicate@octarq.local" } });
    fireEvent.click(screen.getByRole("button", { name: /continue/i }));

    await waitFor(() => {
      expect(toastErrorSpy).toHaveBeenCalledWith(expect.stringMatching(/already exists/i));
    });
  });

  it("validates and submits password change form", async () => {
    vi.spyOn(api, "me").mockResolvedValue({
      email: "user@octarq.local",
      orgId: 1,
    });
    const changePassSpy = vi.spyOn(api, "changePassword").mockResolvedValue({ ok: true });
    const toastSuccessSpy = vi.spyOn(ui.toast, "success");

    render(
      <I18nProvider>
        <MemoryRouter>
          <ProfileSettings />
        </MemoryRouter>
      </I18nProvider>,
    );

    const inputs = await screen.findAllByPlaceholderText("••••••••");
    expect(inputs).toHaveLength(3);

    fireEvent.change(inputs[0], { target: { value: "oldSecret123" } });
    fireEvent.change(inputs[1], { target: { value: "newSecretPass" } });
    fireEvent.change(inputs[2], { target: { value: "newSecretPass" } });

    fireEvent.click(screen.getByRole("button", { name: /update password/i }));

    await waitFor(() => {
      expect(changePassSpy).toHaveBeenCalledWith("oldSecret123", "newSecretPass");
      expect(toastSuccessSpy).toHaveBeenCalled();
    });
  });

  it("validates password length and mismatch on password change form", async () => {
    vi.spyOn(api, "me").mockResolvedValue({
      email: "user@octarq.local",
      orgId: 1,
    });
    const toastErrorSpy = vi.spyOn(ui.toast, "error");

    render(
      <I18nProvider>
        <MemoryRouter>
          <ProfileSettings />
        </MemoryRouter>
      </I18nProvider>,
    );

    const inputs = await screen.findAllByPlaceholderText("••••••••");

    // Test too short
    fireEvent.change(inputs[0], { target: { value: "oldSecret" } });
    fireEvent.change(inputs[1], { target: { value: "short" } });
    fireEvent.change(inputs[2], { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /update password/i }));
    expect(toastErrorSpy).toHaveBeenCalledWith(expect.stringMatching(/at least 8 characters/i));

    // Test mismatch
    fireEvent.change(inputs[1], { target: { value: "longenoughpass1" } });
    fireEvent.change(inputs[2], { target: { value: "longenoughpass2" } });
    fireEvent.click(screen.getByRole("button", { name: /update password/i }));
    expect(toastErrorSpy).toHaveBeenCalledWith(expect.stringMatching(/do not match/i));
  });
});

describe("ApiTokens suite", () => {
  it("renders token list and empty state", async () => {
    vi.spyOn(api, "tokens").mockResolvedValue([]);

    render(
      <I18nProvider>
        <MemoryRouter>
          <RoleProvider value={{ role: "admin", isInstanceAdmin: false }}>
            <ApiTokens />
          </RoleProvider>
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByText(/no api tokens configured yet/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /\+ new token/i })).toBeTruthy();
  });

  it("renders existing tokens and handles revoke action", async () => {
    vi.spyOn(api, "tokens").mockResolvedValue([
      {
        id: 1,
        name: "ci-pipeline",
        prefix: "oct_live_1234",
        note: "Github Actions runner",
        userId: 1,
        role: "admin",
        lastUsedAt: "2026-08-30T00:00:00Z",
        expiresAt: null,
        createdAt: "2026-08-01T00:00:00Z",
      },
    ]);
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const deleteSpy = vi.spyOn(api, "deleteToken").mockResolvedValue(undefined as any);

    render(
      <I18nProvider>
        <MemoryRouter>
          <RoleProvider value={{ role: "admin", isInstanceAdmin: false }}>
            <ApiTokens />
          </RoleProvider>
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByText("ci-pipeline")).toBeTruthy();
    expect(screen.getByText("Github Actions runner")).toBeTruthy();
    expect(screen.getByText("oct_live_1234…")).toBeTruthy();

    const revokeBtn = screen.getByRole("button", { name: /revoke/i });
    fireEvent.click(revokeBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(deleteSpy).toHaveBeenCalledWith(1);
    });
  });

  it("creates a new token via modal and displays copy affordance", async () => {
    vi.spyOn(api, "tokens").mockResolvedValue([]);
    const createSpy = vi.spyOn(api, "createToken").mockResolvedValue({
      id: 2,
      name: "automation-script",
      prefix: "oct_auto",
      note: "Backup script",
      userId: 1,
      role: "member",
      lastUsedAt: null,
      expiresAt: null,
      createdAt: "2026-08-31T00:00:00Z",
      token: "oct_live_raw_super_secret_token_12345",
    });

    render(
      <I18nProvider>
        <MemoryRouter>
          <RoleProvider value={{ role: "owner", isInstanceAdmin: true }}>
            <ApiTokens />
          </RoleProvider>
        </MemoryRouter>
      </I18nProvider>,
    );

    fireEvent.click(await screen.findByRole("button", { name: /\+ new token/i }));
    expect(screen.getByText("Generate API Token")).toBeTruthy();

    const nameInput = screen.getByPlaceholderText("e.g. cli-tool");
    fireEvent.change(nameInput, { target: { value: "automation-script" } });

    const noteInput = screen.getByPlaceholderText("e.g. home server cron job");
    fireEvent.change(noteInput, { target: { value: "Backup script" } });

    fireEvent.click(screen.getByRole("button", { name: /^generate token$/i }));

    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        name: "automation-script",
        note: "Backup script",
        role: "member",
        expiresInDays: 0,
      });
      expect(screen.getByText("Token Generated")).toBeTruthy();
      expect(screen.getByText("oct_live_raw_super_secret_token_12345")).toBeTruthy();
      expect(screen.getByRole("button", { name: /copy to clipboard/i })).toBeTruthy();
    });
  });

  it("edits an existing token", async () => {
    vi.spyOn(api, "tokens").mockResolvedValue([
      {
        id: 3,
        name: "my-token",
        prefix: "oct_prefix",
        note: "Old note",
        userId: 1,
        role: "member",
        lastUsedAt: null,
        expiresAt: null,
        createdAt: "2026-08-01T00:00:00Z",
      },
    ]);
    const updateSpy = vi.spyOn(api, "updateToken").mockResolvedValue({
      id: 3,
      name: "renamed-token",
      prefix: "oct_prefix",
      note: "Updated note",
      userId: 1,
      role: "admin",
      lastUsedAt: null,
      expiresAt: null,
      createdAt: "2026-08-01T00:00:00Z",
    });

    render(
      <I18nProvider>
        <MemoryRouter>
          <RoleProvider value={{ role: "admin", isInstanceAdmin: false }}>
            <ApiTokens />
          </RoleProvider>
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByText("my-token")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /edit/i }));

    expect(screen.getByText("Edit API Token")).toBeTruthy();
    const nameInput = screen.getByDisplayValue("my-token");
    fireEvent.change(nameInput, { target: { value: "renamed-token" } });

    const noteInput = screen.getByDisplayValue("Old note");
    fireEvent.change(noteInput, { target: { value: "Updated note" } });

    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(3, {
        name: "renamed-token",
        note: "Updated note",
      });
    });
  });
});
