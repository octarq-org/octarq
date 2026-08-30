// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, cleanup, waitFor, fireEvent } from "@testing-library/react";
import { api, ApiError } from "../../api";
import { I18nProvider } from "../../i18n";
import * as ui from "../../ui";
import { GeneralSettings } from "./general";

function renderGeneralSettings() {
  return render(
    <I18nProvider>
      <GeneralSettings />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("GeneralSettings suite", () => {
  it("loads and displays workspace profile and tenant subdomain", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme", role: "owner" },
    ]);
    vi.spyOn(api, "orgSlug").mockResolvedValue({ slug: "acme" });
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme",
      tenantSubdomain: "acme.octarq.io",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });

    renderGeneralSettings();

    expect(await screen.findByDisplayValue("Acme Corp")).toBeTruthy();
    expect(await screen.findByText("acme.octarq.io")).toBeTruthy();
    expect(await screen.findByDisplayValue("acme")).toBeTruthy();
  });

  it("updates workspace name and dispatches window event", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme", role: "owner" },
    ]);
    vi.spyOn(api, "orgSlug").mockResolvedValue({ slug: "acme" });
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });
    const updateOrgSpy = vi.spyOn(api, "updateOrg").mockResolvedValue({
      id: 1,
      name: "Acme Global",
      slug: "acme",
    });
    const toastSpy = vi.spyOn(ui.toast, "success");
    const eventSpy = vi.fn();
    window.addEventListener("octarq:orgs-changed", eventSpy);

    renderGeneralSettings();

    const input = await screen.findByDisplayValue("Acme Corp");
    fireEvent.change(input, { target: { value: "Acme Global" } });
    const updateButtons = screen.getAllByRole("button", { name: /update/i });
    fireEvent.click(updateButtons[0]);

    await waitFor(() => {
      expect(updateOrgSpy).toHaveBeenCalledWith({ name: "Acme Global" });
      expect(toastSpy).toHaveBeenCalled();
      expect(eventSpy).toHaveBeenCalled();
    });

    window.removeEventListener("octarq:orgs-changed", eventSpy);
  });

  it("updates workspace address slug through confirmation modal", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "owner@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme-old", role: "owner" },
    ]);
    vi.spyOn(api, "orgSlug").mockResolvedValue({ slug: "acme-old" });
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme-old",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });
    const updateSlugSpy = vi.spyOn(api, "updateOrgSlug").mockResolvedValue({ slug: "acme-new" });
    const toastSpy = vi.spyOn(ui.toast, "success");

    renderGeneralSettings();

    const slugInput = await screen.findByDisplayValue("acme-old");
    fireEvent.change(slugInput, { target: { value: "acme-new" } });

    const updateButtons = screen.getAllByRole("button", { name: /update/i });
    fireEvent.click(updateButtons[1]); // The slug form update button

    expect(await screen.findByText(/change the workspace address\?/i)).toBeTruthy();
    expect(screen.getByText(/\/api\/webhook\/acme-new\/billing/i)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /change address/i }));

    await waitFor(() => {
      expect(updateSlugSpy).toHaveBeenCalledWith("acme-new");
      expect(toastSpy).toHaveBeenCalled();
    });
  });

  it("handles duplicate slug error on workspace address update", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "owner@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme-old", role: "owner" },
    ]);
    vi.spyOn(api, "orgSlug").mockResolvedValue({ slug: "acme-old" });
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme-old",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });
    vi.spyOn(api, "updateOrgSlug").mockRejectedValue(new ApiError(409, "Slug already in use"));
    const toastErrorSpy = vi.spyOn(ui.toast, "error");

    renderGeneralSettings();

    const slugInput = await screen.findByDisplayValue("acme-old");
    fireEvent.change(slugInput, { target: { value: "duplicate-slug" } });

    const updateButtons = screen.getAllByRole("button", { name: /update/i });
    fireEvent.click(updateButtons[1]);

    fireEvent.click(await screen.findByRole("button", { name: /change address/i }));

    await waitFor(() => {
      expect(toastErrorSpy).toHaveBeenCalledWith("Slug already in use");
    });
  });

  it("exports workspace data on button click", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "admin@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme", role: "admin" },
    ]);
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });
    const exportSpy = vi.spyOn(api, "exportWorkspaceData").mockResolvedValue({
      org: { id: 1, name: "Acme Corp" },
      links: [],
    });

    renderGeneralSettings();

    const exportBtn = await screen.findByRole("button", { name: /download my data/i });
    fireEvent.click(exportBtn);

    await waitFor(() => {
      expect(exportSpy).toHaveBeenCalled();
    });
  });

  it("purges workspace data after typing confirmation phrase", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ email: "owner@acme.com", orgId: 1 });
    vi.spyOn(api, "orgs").mockResolvedValue([
      { id: 1, name: "Acme Corp", slug: "acme", role: "owner" },
    ]);
    vi.spyOn(api, "orgSlug").mockResolvedValue({ slug: "acme" });
    vi.spyOn(api, "settings").mockResolvedValue({
      reservedMailboxes: "",
      orgSlug: "acme",
      catchAll: false,
      autoWrapLinks: false,
      isInstanceAdmin: false,
    });
    const purgeSpy = vi.spyOn(api, "purgeWorkspaceData").mockResolvedValue(undefined as any);
    const toastSuccessSpy = vi.spyOn(ui.toast, "success");

    renderGeneralSettings();

    const deleteBtn = await screen.findByRole("button", { name: /delete workspace/i });
    fireEvent.click(deleteBtn);

    expect(await screen.findByText(/delete this workspace\?/i)).toBeTruthy();

    const confirmInput = screen.getByPlaceholderText("DELETE MY DATA");
    fireEvent.change(confirmInput, { target: { value: "DELETE MY DATA" } });

    const deleteConfirmBtn = screen.getByRole("button", { name: /permanently delete/i });
    expect(deleteConfirmBtn.getAttribute("disabled")).toBeNull();

    fireEvent.click(deleteConfirmBtn);

    await waitFor(() => {
      expect(purgeSpy).toHaveBeenCalled();
      expect(toastSuccessSpy).toHaveBeenCalled();
    });
  });
});
