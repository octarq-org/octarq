// @vitest-environment happy-dom
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

    expect(await screen.findByText(/check your email/i)).toBeTruthy();
    expect(screen.getByText(/new@user\.com/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /resend email/i })).toBeTruthy();
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

    expect(screen.queryByText(/email sent/i)).toBeNull();
    expect(await screen.findByText(/email delivery is not configured/i)).toBeTruthy();
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

    expect(await screen.findByText(/email sent/i)).toBeTruthy();
    expect(screen.queryByText(/email delivery is not configured/i)).toBeNull();
  });
});
