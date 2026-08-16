// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import InstanceConsole from "./console";
import { I18nProvider } from "../../i18n";
import { api } from "../../api";

vi.mock("../../api", () => ({
  ApiError: class ApiError extends Error {},
  api: {
    me: vi.fn(),
    authConfig: vi.fn().mockResolvedValue({}),
    instanceBuild: vi.fn().mockResolvedValue({ version: "dev", commit: "unknown", builtAt: "unknown" }),
    instanceReadiness: vi.fn(),
  },
}));

function renderConsole(entry = "/") {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <I18nProvider>
        <InstanceConsole />
      </I18nProvider>
    </MemoryRouter>,
  );
}

const FIXABLE_READINESS = [
  { id: "public-origin", status: "degraded", title: "public origin", detail: "no domain registered", fixPath: "/domains" },
  { id: "outbound-mail", status: "ok", title: "outbound mail", detail: "system sender available", fixPath: "/mail?tab=settings" },
  { id: "registration", status: "blocked", title: "registration", detail: "sign-up dead end", fixPath: "/mail?tab=settings" },
  { id: "database", status: "ok", title: "database", detail: "driver=sqlite", fixPath: "" },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("InstanceConsole gate", () => {
  it("shows a neutral notice to non-instance-admins and mounts no instance features", async () => {
    (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      email: "user@example.com",
      orgId: 1,
      isInstanceAdmin: false,
    });

    renderConsole();

    await waitFor(() => {
      expect(screen.getByText("This page isn't available to your account")).toBeTruthy();
    });
    // No readiness fetch, no wizard, no console rail — nothing instance-level.
    expect(api.instanceReadiness).not.toHaveBeenCalled();
    expect(api.instanceBuild).not.toHaveBeenCalled();
    expect(screen.queryByText("Set up this instance")).toBeNull();
    expect(screen.queryByText("Instance health")).toBeNull();
  });
});

describe("Setup wizard derivation", () => {
  it("derives steps from the readiness report and renders a blocked step as blocked", async () => {
    (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      email: "admin@example.com",
      orgId: 1,
      isInstanceAdmin: true,
    });
    (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(FIXABLE_READINESS);

    renderConsole("/");

    // The report drives the UI: the blocked check renders with a distinct
    // blocked state, the degraded one with degraded, the ok one with ok.
    await waitFor(() => {
      expect(document.querySelector('[data-state="blocked"]')).toBeTruthy();
    });
    expect(document.querySelector('[data-state="degraded"]')).toBeTruthy();
    expect(document.querySelector('[data-state="ok"]')).toBeTruthy();
    // Only fixable checks become wizard steps — database (no fixPath) doesn't.
    expect(document.querySelectorAll("[data-state]").length).toBe(3);

    // Blocked steps carry a fix action that jumps into the dashboard.
    const blockedFix = document.querySelector(
      '[data-state="blocked"] a[href="/admin/mail?tab=settings"]',
    );
    expect(blockedFix).toBeTruthy();

    // The blocked banner names the count.
    expect(screen.getByText(/1 blocked item/)).toBeTruthy();
    // The blocked badge is the explicit "Blocked" label, distinct from
    // degraded's — the two states must not read the same.
    expect(screen.getByText("Blocked")).toBeTruthy();
    expect(screen.getByText("Degraded")).toBeTruthy();
  });
});
