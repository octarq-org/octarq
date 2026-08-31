// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api, ApiError } from "../../api";
import { I18nProvider } from "../../i18n";
import { RoleProvider } from "../../shell/role";
import * as ui from "../../ui";
import { OrgMembersManager } from "./members";

function renderMembersManager(role = "admin", isInstanceAdmin = false) {
  return render(
    <I18nProvider>
      <RoleProvider value={{ role, isInstanceAdmin }}>
        <OrgMembersManager />
      </RoleProvider>
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("OrgMembersManager suite", () => {
  it("renders member list with pending and joined statuses", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgMembers").mockResolvedValue([
      { userId: 1, email: "admin@acme.com", role: "owner", joinedAt: "2026-01-01T00:00:00Z" },
      { userId: 2, email: "colleague@acme.com", role: "member", pending: true },
    ]);

    renderMembersManager("owner", true);

    expect(await screen.findByText("admin@acme.com")).toBeTruthy();
    expect(screen.getByText("colleague@acme.com")).toBeTruthy();
    expect(screen.getByText("Pending")).toBeTruthy();
  });

  it("invites new member and handles success toast", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgMembers").mockResolvedValue([
      { userId: 1, email: "admin@acme.com", role: "owner" },
    ]);
    const addMemberSpy = vi.spyOn(api, "addOrgMember").mockResolvedValue({ ok: true, emailSent: true });
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderMembersManager("admin", false);

    const emailInput = await screen.findByPlaceholderText("colleague@example.com");
    fireEvent.change(emailInput, { target: { value: "newbie@acme.com" } });
    fireEvent.click(screen.getByRole("button", { name: /invite member/i }));

    await waitFor(() => {
      expect(addMemberSpy).toHaveBeenCalledWith({ email: "newbie@acme.com", role: "member" });
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("shows warning toast when email service is unconfigured on invite", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgMembers").mockResolvedValue([
      { userId: 1, email: "admin@acme.com", role: "owner" },
    ]);
    vi.spyOn(api, "addOrgMember").mockResolvedValue({ ok: true, emailSent: false });
    const toastWarningSpy = vi.spyOn(ui.toast, "warning");

    renderMembersManager("admin", false);

    const emailInput = await screen.findByPlaceholderText("colleague@example.com");
    fireEvent.change(emailInput, { target: { value: "nomail@acme.com" } });
    fireEvent.click(screen.getByRole("button", { name: /invite member/i }));

    await waitFor(() => {
      expect(toastWarningSpy).toHaveBeenCalled();
    });
  });

  it("removes a member after confirmation", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgMembers").mockResolvedValue([
      { userId: 1, email: "admin@acme.com", role: "owner" },
      { userId: 2, email: "toremove@acme.com", role: "member" },
    ]);
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    const deleteSpy = vi.spyOn(api, "deleteOrgMember").mockResolvedValue(undefined as any);
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderMembersManager("admin", false);

    expect(await screen.findByText("toremove@acme.com")).toBeTruthy();
    const removeBtn = screen.getByRole("button", { name: /remove/i });
    fireEvent.click(removeBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalled();
      expect(deleteSpy).toHaveBeenCalledWith(2);
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("restricts member management for standard member role", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "plain@acme.com", orgId: 1 });
    vi.spyOn(api, "orgMembers").mockResolvedValue([
      { userId: 1, email: "admin@acme.com", role: "admin" },
      { userId: 2, email: "plain@acme.com", role: "member" },
    ]);

    renderMembersManager("member", false);

    expect(await screen.findByText("plain@acme.com")).toBeTruthy();
    expect(screen.queryByPlaceholderText("colleague@example.com")).toBeNull();
    expect(screen.queryByRole("button", { name: /invite member/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /remove/i })).toBeNull();
  });
});
