// @vitest-environment happy-dom
//
// Guards the sign-up half of the email-verification gate in the REAL Login
// screen. When the instance requires a verified email the server creates the
// account but issues no session (internal/api/register.go), flagging it with
// `verificationRequired`. The UI must branch on that flag: show "verify your
// email", not push the user at a dashboard they can't load.
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
      verificationRequired: true,
    });

    await signUp();

    // The address is echoed back so the user knows which mailbox to open.
    expect(await screen.findByText(/verify your email/i)).toBeTruthy();
    expect(screen.getByText(/new@user\.com/)).toBeTruthy();
    // A resend action is reachable from this state, not just from a failed login.
    expect(screen.getByRole("button", { name: /resend verification email/i })).toBeTruthy();
    // No session exists, so nothing may try to enter the app.
    expect(onLogin).not.toHaveBeenCalled();
    expect(me).not.toHaveBeenCalled();
  });

  it("still logs the user straight in when the gate is off", async () => {
    const onLogin = renderLogin();
    vi.spyOn(api, "me").mockResolvedValue({ email: "new@user.com", orgId: 7 });
    vi.spyOn(api, "register").mockResolvedValue({
      ok: true,
      email: "new@user.com",
      verificationRequired: false,
    });

    await signUp();

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith("new@user.com", 7));
    expect(screen.queryByText(/verify your email/i)).toBeNull();
  });
});
