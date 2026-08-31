// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api, ApiError } from "../api";
import { I18nProvider } from "../i18n";
import { Login } from "./Login";

function renderLogin(onLogin = vi.fn()) {
  vi.spyOn(api, "authConfig").mockResolvedValue({
    googleEnabled: true,
    githubEnabled: true,
    registrationEnabled: true,
    appName: "Octarq",
    logoUrl: "",
    brandColor: "",
    brandColor2: "",
  });
  render(
    <I18nProvider>
      <Login onLogin={onLogin} />
    </I18nProvider>,
  );
  return onLogin;
}

beforeEach(() => {
  window.history.replaceState({}, "", "/");
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("Login form suite", () => {
  it("renders login form with email, password, and sign-in button", async () => {
    renderLogin();

    expect(await screen.findByLabelText(/email/i)).toBeTruthy();
    expect(screen.getByLabelText(/password/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /^sign in$/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /forgot password\?/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /create one/i })).toBeTruthy();
    expect(screen.getByText("Google")).toBeTruthy();
    expect(screen.getByText("GitHub")).toBeTruthy();
  });

  it("submits valid credentials and calls onLogin", async () => {
    const onLogin = renderLogin();
    const loginSpy = vi.spyOn(api, "login").mockResolvedValue({
      ok: true,
      email: "user@example.com",
    });
    const meSpy = vi.spyOn(api, "me").mockResolvedValue({
      email: "user@example.com",
      orgId: 42,
    });

    const emailInput = await screen.findByLabelText(/email/i);
    const passInput = screen.getByLabelText(/password/i);

    fireEvent.change(emailInput, { target: { value: "user@example.com" } });
    fireEvent.change(passInput, { target: { value: "mypassword123" } });
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));

    await waitFor(() => {
      expect(loginSpy).toHaveBeenCalledWith("user@example.com", "mypassword123");
      expect(meSpy).toHaveBeenCalled();
      expect(onLogin).toHaveBeenCalledWith("user@example.com", 42);
    });
  });

  it("switches to OTP form when 2FA is required and verifies code", async () => {
    const onLogin = renderLogin();
    vi.spyOn(api, "login").mockResolvedValue({
      twoFactorRequired: true,
      email: "2fa@user.com",
    });
    const verify2FASpy = vi.spyOn(api, "verify2FA").mockResolvedValue({ ok: true });
    vi.spyOn(api, "me").mockResolvedValue({
      email: "2fa@user.com",
      orgId: 10,
    });

    fireEvent.change(await screen.findByLabelText(/email/i), { target: { value: "2fa@user.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "pass123" } });
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));

    expect(await screen.findByLabelText(/authentication code/i)).toBeTruthy();
    const otpInput = screen.getByLabelText(/authentication code/i);
    fireEvent.change(otpInput, { target: { value: "123456" } });
    fireEvent.click(screen.getByRole("button", { name: /verify otp/i }));

    await waitFor(() => {
      expect(verify2FASpy).toHaveBeenCalledWith("2fa@user.com", "pass123", "123456");
      expect(onLogin).toHaveBeenCalledWith("2fa@user.com", 10);
    });
  });

  it("handles OAuth pending 2FA from URL parameter (?twofa=1)", async () => {
    window.history.replaceState({}, "", "/?twofa=1");
    const onLogin = renderLogin();
    const verifyChallengeSpy = vi.spyOn(api, "verify2FAChallenge").mockResolvedValue({ ok: true });
    vi.spyOn(api, "me").mockResolvedValue({
      email: "oauth@user.com",
      orgId: 5,
    });

    expect(await screen.findByLabelText(/authentication code/i)).toBeTruthy();
    const otpInput = screen.getByLabelText(/authentication code/i);
    fireEvent.change(otpInput, { target: { value: "654321" } });
    fireEvent.click(screen.getByRole("button", { name: /verify otp/i }));

    await waitFor(() => {
      expect(verifyChallengeSpy).toHaveBeenCalledWith("654321");
      expect(onLogin).toHaveBeenCalledWith("admin", 5);
    });
  });

  it("displays error message on invalid credentials", async () => {
    renderLogin();
    vi.spyOn(api, "login").mockRejectedValue(new ApiError(401, "Invalid email or password"));

    fireEvent.change(await screen.findByLabelText(/email/i), { target: { value: "wrong@user.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "badpass" } });
    fireEvent.click(screen.getByRole("button", { name: /^sign in$/i }));

    expect(await screen.findByText("Invalid email or password")).toBeTruthy();
  });

  it("shows disabled and loading state on submit button while request is in-flight", async () => {
    renderLogin();
    let resolveLogin!: (val: any) => void;
    vi.spyOn(api, "login").mockImplementation(() => new Promise((resolve) => { resolveLogin = resolve; }));

    fireEvent.change(await screen.findByLabelText(/email/i), { target: { value: "test@user.com" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "pass" } });
    const submitBtn = screen.getByRole("button", { name: /^sign in$/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(submitBtn.getAttribute("disabled")).not.toBeNull();
      expect(screen.getByRole("button", { name: /signing in/i })).toBeTruthy();
    });

    resolveLogin({ ok: true, email: "test@user.com" });
    vi.spyOn(api, "me").mockResolvedValue({ email: "test@user.com", orgId: 1 });
  });

  it("supports forgot password flow and switches back to sign in", async () => {
    renderLogin();
    const forgotSpy = vi.spyOn(api, "forgotPassword").mockResolvedValue({ ok: true });

    fireEvent.click(await screen.findByRole("button", { name: /forgot password\?/i }));
    expect(screen.getByText(/enter your account email to receive a reset link/i)).toBeTruthy();

    const emailInput = screen.getByLabelText(/email/i);
    fireEvent.change(emailInput, { target: { value: "recover@user.com" } });
    fireEvent.click(screen.getByRole("button", { name: /send reset link/i }));

    await waitFor(() => {
      expect(forgotSpy).toHaveBeenCalledWith("recover@user.com");
      expect(screen.getByText(/if an account exists with that email, a reset link has been sent/i)).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: /back to sign in/i }));
    expect(screen.getByRole("button", { name: /^sign in$/i })).toBeTruthy();
  });

  it("supports register mode and completes registration", async () => {
    const onLogin = renderLogin();
    const regSpy = vi.spyOn(api, "register").mockResolvedValue({
      ok: true,
      email: "newreg@user.com",
      verificationRequired: false,
    });
    vi.spyOn(api, "me").mockResolvedValue({ email: "newreg@user.com", orgId: 99 });

    fireEvent.click(await screen.findByRole("button", { name: /create one/i }));
    expect(screen.getByLabelText(/workspace name/i)).toBeTruthy();

    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "newreg@user.com" } });
    fireEvent.change(screen.getByLabelText(/workspace name/i), { target: { value: "Acme Corp" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "secret123" } });
    fireEvent.click(screen.getByRole("button", { name: /create account/i }));

    await waitFor(() => {
      expect(regSpy).toHaveBeenCalledWith("newreg@user.com", "secret123", "Acme Corp");
      expect(onLogin).toHaveBeenCalledWith("newreg@user.com", 99);
    });
  });
});
