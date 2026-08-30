// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { api, ApiError } from "../api";
import { I18nProvider } from "../i18n";
import InviteAcceptPage from "./InviteAccept";

function renderInviteAccept(path = "/invite/accept") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/invite/accept" element={<InviteAcceptPage />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("InviteAcceptPage suite", () => {
  it("renders warning and disables submit button when token is missing", async () => {
    renderInviteAccept("/invite/accept");

    expect(await screen.findByText(/no token found in url/i)).toBeTruthy();
    const submitBtn = screen.getByRole("button", { name: /activate account/i });
    expect(submitBtn.getAttribute("disabled")).not.toBeNull();
  });

  it("renders password inputs when valid invite token is provided", async () => {
    renderInviteAccept("/invite/accept?token=invite-token-123");

    expect(screen.queryByText(/no token found in url/i)).toBeNull();
    expect(screen.getByText(/^new password$/i)).toBeTruthy();
    expect(screen.getByText(/^confirm password$/i)).toBeTruthy();
    expect(screen.getByPlaceholderText(/at least 8 characters/i)).toBeTruthy();
    expect(screen.getByPlaceholderText(/repeat new password/i)).toBeTruthy();
    const submitBtn = screen.getByRole("button", { name: /activate account/i });
    expect(submitBtn.getAttribute("disabled")).toBeNull();
  });

  it("validates password length is at least 8 characters", async () => {
    renderInviteAccept("/invite/accept?token=invite-token-123");

    const passInput = screen.getByPlaceholderText(/at least 8 characters/i);
    const confirmInput = screen.getByPlaceholderText(/repeat new password/i);

    fireEvent.change(passInput, { target: { value: "short" } });
    fireEvent.change(confirmInput, { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /activate account/i }));

    expect(await screen.findByText(/password must be at least 8 characters long/i)).toBeTruthy();
  });

  it("validates that passwords match", async () => {
    renderInviteAccept("/invite/accept?token=invite-token-123");

    const passInput = screen.getByPlaceholderText(/at least 8 characters/i);
    const confirmInput = screen.getByPlaceholderText(/repeat new password/i);

    fireEvent.change(passInput, { target: { value: "password123" } });
    fireEvent.change(confirmInput, { target: { value: "password456" } });
    fireEvent.click(screen.getByRole("button", { name: /activate account/i }));

    expect(await screen.findByText(/passwords do not match/i)).toBeTruthy();
  });

  it("submits valid password and displays success state", async () => {
    renderInviteAccept("/invite/accept?token=valid-invite-abc");
    const acceptSpy = vi.spyOn(api, "acceptInvite").mockResolvedValue({ ok: true });

    const passInput = screen.getByPlaceholderText(/at least 8 characters/i);
    const confirmInput = screen.getByPlaceholderText(/repeat new password/i);

    fireEvent.change(passInput, { target: { value: "memberPassword123" } });
    fireEvent.change(confirmInput, { target: { value: "memberPassword123" } });
    fireEvent.click(screen.getByRole("button", { name: /activate account/i }));

    await waitFor(() => {
      expect(acceptSpy).toHaveBeenCalledWith("valid-invite-abc", "memberPassword123");
      expect(screen.getByText(/account activated/i)).toBeTruthy();
      expect(screen.getByText(/your account is ready/i)).toBeTruthy();
    });
  });

  it("displays server error message when invite accept fails", async () => {
    renderInviteAccept("/invite/accept?token=expired-invite");
    vi.spyOn(api, "acceptInvite").mockRejectedValue(new ApiError(400, "Invitation token expired or invalid"));

    const passInput = screen.getByPlaceholderText(/at least 8 characters/i);
    const confirmInput = screen.getByPlaceholderText(/repeat new password/i);

    fireEvent.change(passInput, { target: { value: "memberPassword123" } });
    fireEvent.change(confirmInput, { target: { value: "memberPassword123" } });
    fireEvent.click(screen.getByRole("button", { name: /activate account/i }));

    expect(await screen.findByText("Invitation token expired or invalid")).toBeTruthy();
  });

  it("shows loading and disabled state while submitting", async () => {
    renderInviteAccept("/invite/accept?token=valid-invite");
    let resolveAccept!: (val: any) => void;
    vi.spyOn(api, "acceptInvite").mockImplementation(() => new Promise((resolve) => { resolveAccept = resolve; }));

    const passInput = screen.getByPlaceholderText(/at least 8 characters/i);
    const confirmInput = screen.getByPlaceholderText(/repeat new password/i);

    fireEvent.change(passInput, { target: { value: "memberPassword123" } });
    fireEvent.change(confirmInput, { target: { value: "memberPassword123" } });
    const submitBtn = screen.getByRole("button", { name: /activate account/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(submitBtn.getAttribute("disabled")).not.toBeNull();
      expect(screen.getByRole("button", { name: /activating account\.\.\./i })).toBeTruthy();
    });

    resolveAccept({ ok: true });
  });
});
