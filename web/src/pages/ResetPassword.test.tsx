// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { api, ApiError } from "../api";
import { I18nProvider } from "../i18n";
import ResetPasswordPage from "./ResetPassword";

function renderResetPassword(path = "/reset-password") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/reset-password" element={<ResetPasswordPage />} />
        </Routes>
      </MemoryRouter>
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("ResetPasswordPage suite", () => {
  it("renders warning and disables submit button when token is missing", async () => {
    renderResetPassword("/reset-password");

    expect(await screen.findByText(/invalid or missing password reset link/i)).toBeTruthy();
    const submitBtn = screen.getByRole("button", { name: /reset password/i });
    expect(submitBtn.getAttribute("disabled")).not.toBeNull();
  });

  it("renders password inputs when valid token is provided", async () => {
    renderResetPassword("/reset-password?token=valid-token-123");

    expect(screen.queryByText(/invalid or missing password reset link/i)).toBeNull();
    expect(screen.getByText(/^new password$/i)).toBeTruthy();
    expect(screen.getByText(/^confirm new password$/i)).toBeTruthy();
    const inputs = screen.getAllByPlaceholderText("••••••••");
    expect(inputs).toHaveLength(2);
    const submitBtn = screen.getByRole("button", { name: /reset password/i });
    expect(submitBtn.getAttribute("disabled")).toBeNull();
  });

  it("validates password length is at least 8 characters", async () => {
    renderResetPassword("/reset-password?token=valid-token-123");

    const inputs = screen.getAllByPlaceholderText("••••••••");
    fireEvent.change(inputs[0], { target: { value: "short" } });
    fireEvent.change(inputs[1], { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /reset password/i }));

    expect(await screen.findByText(/password must be at least 8 characters/i)).toBeTruthy();
  });

  it("validates that passwords match", async () => {
    renderResetPassword("/reset-password?token=valid-token-123");

    const inputs = screen.getAllByPlaceholderText("••••••••");
    fireEvent.change(inputs[0], { target: { value: "password123" } });
    fireEvent.change(inputs[1], { target: { value: "password456" } });
    fireEvent.click(screen.getByRole("button", { name: /reset password/i }));

    expect(await screen.findByText(/passwords do not match/i)).toBeTruthy();
  });

  it("submits valid new password and displays success state", async () => {
    renderResetPassword("/reset-password?token=test-token-xyz");
    const resetSpy = vi.spyOn(api, "resetPassword").mockResolvedValue({ ok: true });

    const inputs = screen.getAllByPlaceholderText("••••••••");
    fireEvent.change(inputs[0], { target: { value: "brandNewPass123" } });
    fireEvent.change(inputs[1], { target: { value: "brandNewPass123" } });
    fireEvent.click(screen.getByRole("button", { name: /reset password/i }));

    await waitFor(() => {
      expect(resetSpy).toHaveBeenCalledWith("test-token-xyz", "brandNewPass123");
      expect(screen.getByText(/password reset complete/i)).toBeTruthy();
      expect(screen.getByText(/your password has been updated/i)).toBeTruthy();
    });
  });

  it("displays server error message when reset fails", async () => {
    renderResetPassword("/reset-password?token=expired-token");
    vi.spyOn(api, "resetPassword").mockRejectedValue(new ApiError(400, "Reset token has expired or is invalid"));

    const inputs = screen.getAllByPlaceholderText("••••••••");
    fireEvent.change(inputs[0], { target: { value: "brandNewPass123" } });
    fireEvent.change(inputs[1], { target: { value: "brandNewPass123" } });
    fireEvent.click(screen.getByRole("button", { name: /reset password/i }));

    expect(await screen.findByText("Reset token has expired or is invalid")).toBeTruthy();
  });

  it("shows loading and disabled state while submitting", async () => {
    renderResetPassword("/reset-password?token=valid-token");
    let resolveReset!: (val: any) => void;
    vi.spyOn(api, "resetPassword").mockImplementation(() => new Promise((resolve) => { resolveReset = resolve; }));

    const inputs = screen.getAllByPlaceholderText("••••••••");
    fireEvent.change(inputs[0], { target: { value: "brandNewPass123" } });
    fireEvent.change(inputs[1], { target: { value: "brandNewPass123" } });
    const submitBtn = screen.getByRole("button", { name: /reset password/i });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(submitBtn.getAttribute("disabled")).not.toBeNull();
      expect(screen.getByRole("button", { name: /resetting…/i })).toBeTruthy();
    });

    resolveReset({ ok: true });
  });
});
