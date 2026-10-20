// @vitest-environment happy-dom
//
// Guards the sign-up half of the email-verification gate in the REAL Login
// screen. When the instance requires a verified email the server creates the
// account but issues no session (internal/api/register.go), so the UI must
// send the user to their mailbox instead of pushing them at a dashboard they
// can't load.
//
// The server's answer is the same whether the address was free or already
// had an account — it must not say which — so the page never asks. What the
// mailbox holds decides what happens next.
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api } from "../api";
import { I18nProvider } from "../i18n";
import { Login } from "./Login";

function renderLogin(onLogin = vi.fn()) {
  vi.spyOn(api, "authConfig").mockResolvedValue({
    googleEnabled: false,
    githubEnabled: false,
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

// Drives the register form the way a person does: switch mode, type, submit.
async function signUp() {
  fireEvent.click(await screen.findByRole("button", { name: /create one/i }));
  fireEvent.change(screen.getByLabelText(/email/i), { target: { value: "new@user.com" } });
  fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "hunter2pw" } });
  fireEvent.click(screen.getByRole("button", { name: /^create account$/i }));
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("sign-up when the instance requires a verified email", () => {
  it("renders the pending-verification state instead of entering the app", async () => {
    const onLogin = renderLogin();
    const me = vi.spyOn(api, "me");
    vi.spyOn(api, "register").mockResolvedValue({
      ok: true,
      email: "new@user.com",
    });

    await signUp();

    // The address is echoed back so the user knows which mailbox to open.
    expect(await screen.findByText(/check your email/i)).toBeTruthy();
    expect(screen.getByText(/new@user\.com/)).toBeTruthy();
    // A resend action is reachable from this state, not just from a failed login.
    expect(screen.getByRole("button", { name: /resend email/i })).toBeTruthy();
    // No session exists, so nothing may try to enter the app.
    expect(onLogin).not.toHaveBeenCalled();
    expect(me).not.toHaveBeenCalled();
  });

  it("does not render success notice when resendVerification returns mailConfigured: false", async () => {
    renderLogin();
    vi.spyOn(api, "register").mockResolvedValue({
      ok: true,
      email: "new@user.com",
    });
    const resendSpy = vi.spyOn(api, "resendVerification").mockResolvedValue({
      ok: true,
      mailConfigured: false,
    });

    await signUp();

    const resendBtn = await screen.findByRole("button", { name: /resend email/i });
    fireEvent.click(resendBtn);

    await waitFor(() => expect(resendSpy).toHaveBeenCalledWith("new@user.com"));

    // Success notice must NOT be rendered
    expect(screen.queryByText(/email sent/i)).toBeNull();
    // Error banner must display the unconfigured mail notice
    expect(await screen.findByText(/mail service is not configured/i)).toBeTruthy();
  });

  it("renders success notice when resendVerification returns mailConfigured: true", async () => {
    renderLogin();
    vi.spyOn(api, "register").mockResolvedValue({
      ok: true,
      email: "new@user.com",
    });
    const resendSpy = vi.spyOn(api, "resendVerification").mockResolvedValue({
      ok: true,
      mailConfigured: true,
    });

    await signUp();

    const resendBtn = await screen.findByRole("button", { name: /resend email/i });
    fireEvent.click(resendBtn);

    await waitFor(() => expect(resendSpy).toHaveBeenCalledWith("new@user.com"));

    // Success notice MUST be rendered
    expect(await screen.findByText(/email sent/i)).toBeTruthy();
    // Error notice must NOT be displayed
    expect(screen.queryByText(/mail service is not configured/i)).toBeNull();
  });
});
