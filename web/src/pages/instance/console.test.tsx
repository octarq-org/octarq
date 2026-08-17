// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { lazy } from "react";
import { registerUIPlugin, resetRegistry } from "@octarq/plugin-sdk";
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
    instanceMenus: vi.fn().mockResolvedValue([]),
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
  // No vitest globals in this suite, so RTL's auto-cleanup never runs — unmount
  // explicitly or prior renders leak into the next case. The instanceMenus
  // implementation is reset here too: clearAllMocks clears call history, not
  // the mockResolvedValue each case stacks on top.
  cleanup();
  (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
  resetRegistry();
});

// An admin session for the instance-scope tests below; readiness reports a
// healthy instance so the wizard stays out of the way.
function adminSession() {
  (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
    email: "admin@example.com",
    orgId: 1,
    isInstanceAdmin: true,
  });
  (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
}

// The frontend half of a plugin registering an instance page, like
// examples/plugin-hello's instanceRoutes.
const INSTANCE_MENU = {
  id: "hello-instance",
  label: "Hello (instance)",
  path: "/hello",
  icon: "sparkles",
  category: "",
  order: 1,
};

function registerHelloInstancePage() {
  registerUIPlugin({
    name: "hello",
    routes: [],
    instanceRoutes: [
      { path: "/hello", Component: lazy(() => Promise.resolve({ default: () => <div>Instance-level page</div> })) },
    ],
  });
}

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

describe("Instance console plugin seam", () => {
  it("shows a plugin rail entry and renders its page when the backend announces it AND the frontend registers it", async () => {
    adminSession();
    (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([INSTANCE_MENU]);
    registerHelloInstancePage();

    renderConsole("/");

    // The rail entry appears after the console's own static entries.
    await waitFor(() => {
      expect(screen.getByText("Hello (instance)")).toBeTruthy();
    });

    // Clicking it renders the plugin's instance page.
    fireEvent.click(screen.getByText("Hello (instance)"));
    await waitFor(() => {
      expect(screen.getByText("Instance-level page")).toBeTruthy();
    });
  });

  it("hides a backend-announced entry when no frontend route is registered", async () => {
    adminSession();
    (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([INSTANCE_MENU]);
    // No registerHelloInstancePage() — the plugin's frontend half isn't composed.

    renderConsole("/");

    await waitFor(() => {
      expect(api.instanceMenus).toHaveBeenCalled();
    });
    expect(screen.queryByText("Hello (instance)")).toBeNull();
  });

  it("hides a frontend-registered entry the backend does not announce", async () => {
    adminSession();
    // instanceMenus stays empty — the plugin is disabled or not in this build.
    registerHelloInstancePage();

    // Landing directly on the plugin path must not render its page: without a
    // backend announcement the route is dead, and the catch-all bounces to the
    // console home. waitFor keeps re-checking long enough for a lazily-resolved
    // page chunk to appear if (and only if) the route wrongly exists.
    renderConsole("/hello");

    await waitFor(() => {
      expect(api.instanceMenus).toHaveBeenCalled();
    });
    expect(screen.queryByText("Hello (instance)")).toBeNull();
    await waitFor(() => {
      expect(screen.queryByText("Instance-level page")).toBeNull();
    });
  });
});
