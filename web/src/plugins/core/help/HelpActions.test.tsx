// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import * as ui from "@octarq/plugin-sdk";
import type { Action } from "../../../api";
import { api } from "../../../api";
import { I18nProvider } from "../../../i18n";
import { RoleProvider } from "../../../shell/role";
import { HelpActions } from "./HelpActions";

// @ts-ignore
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const ACTIONS: Action[] = [
  {
    id: "settings.open",
    label: "Open settings",
    description: "Change workspace settings.",
    path: "/settings",
    icon: "settings",
    category: "Settings",
  },
  {
    id: "admin.panel",
    label: "Admin panel",
    description: "Manage members.",
    path: "/admin",
    icon: "shield",
    category: "Admin",
    requiredRole: "admin",
  },
  {
    id: "unrelated.action",
    label: "Unrelated",
    description: "Not on this page.",
    path: "/unrelated",
    icon: "plus",
    category: "Other",
  },
];

const onNavigate = vi.fn();

function renderActions(role: string, isInstanceAdmin = false, ids = ["settings.open", "admin.panel", "missing.action"]) {
  return render(
    <I18nProvider>
      <RoleProvider value={{ role, isInstanceAdmin }}>
        <HelpActions ids={ids} onNavigate={onNavigate} />
      </RoleProvider>
    </I18nProvider>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "actions").mockResolvedValue(ACTIONS);
  onNavigate.mockReset();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("HelpActions", () => {
  it("shows only actions declared by the doc and allowed for the role", async () => {
    renderActions("member");

    expect(await screen.findByText("Open settings")).toBeTruthy();
    expect(screen.getByText("Change workspace settings.")).toBeTruthy();
    expect(screen.queryByText("Admin panel")).toBeNull();
    expect(screen.queryByText("Unrelated")).toBeNull();
  });

  it("shows admin-only actions to admins and instance admins", async () => {
    renderActions("admin");
    expect(await screen.findByText("Admin panel")).toBeTruthy();
    cleanup();

    renderActions("member", true);
    expect(await screen.findByText("Admin panel")).toBeTruthy();
  });

  it("hides an action id that the API does not declare", async () => {
    renderActions("member", false, ["missing.action"]);

    await waitFor(() => expect(api.actions).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("help-actions")).toBeNull();
  });

  it("does not navigate when the confirmation is declined", async () => {
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(false);
    renderActions("member");

    fireEvent.click(await screen.findByRole("button", { name: /Open settings/ }));

    await waitFor(() => expect(confirmSpy).toHaveBeenCalledTimes(1));
    expect(onNavigate).not.toHaveBeenCalled();
  });

  it("navigates after the confirmation is accepted", async () => {
    const confirmSpy = vi.spyOn(ui, "confirmDialog").mockResolvedValue(true);
    renderActions("member");

    fireEvent.click(await screen.findByRole("button", { name: /Open settings/ }));

    await waitFor(() => expect(onNavigate).toHaveBeenCalledWith("/settings"));
    expect(confirmSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        message: "This opens Open settings.",
      }),
    );
  });
});
