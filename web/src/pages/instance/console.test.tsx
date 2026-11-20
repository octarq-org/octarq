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
  cleanup();
  (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
  resetRegistry();
});

function adminSession() {
  (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
    email: "admin@example.com",
    orgId: 1,
    isInstanceAdmin: true,
  });
  (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([]);
}

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
    expect(api.instanceReadiness).not.toHaveBeenCalled();
    expect(api.instanceBuild).not.toHaveBeenCalled();
    expect(screen.queryByText("Set up this instance")).toBeNull();
    expect(screen.queryByText("Instance health")).toBeNull();
  });
});

describe("Console home overview", () => {
  it("renders all readiness checks including unfixable ones when fixable issues exist", async () => {
    (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      email: "admin@example.com",
      orgId: 1,
      isInstanceAdmin: true,
    });
    (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(FIXABLE_READINESS);

    renderConsole("/");

    await waitFor(() => {
      expect(document.querySelector('[data-state="blocked"]')).toBeTruthy();
    }, { timeout: 3000 });
    expect(document.querySelector('[data-state="degraded"]')).toBeTruthy();
    expect(document.querySelector('[data-state="ok"]')).toBeTruthy();
    expect(document.querySelectorAll("[data-state]").length).toBe(4);
    expect(screen.getByText("Database")).toBeTruthy();
    expect(screen.getByText("driver=sqlite")).toBeTruthy();

    const blockedFix = document.querySelector(
      '[data-state="blocked"] a[href="/admin/mail?tab=settings"]',
    );
    expect(blockedFix).toBeTruthy();

    expect(screen.getByText(/1 blocked item/)).toBeTruthy();
    expect(screen.getByText("Blocked")).toBeTruthy();
    expect(screen.getByText("Degraded")).toBeTruthy();
  });

  it("redirects legacy /wizard route to /", async () => {
    (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      email: "admin@example.com",
      orgId: 1,
      isInstanceAdmin: true,
    });
    (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(FIXABLE_READINESS);

    renderConsole("/wizard");

    await waitFor(() => {
      expect(screen.getByText("Instance health")).toBeTruthy();
    }, { timeout: 3000 });
    expect(document.querySelectorAll("[data-state]").length).toBe(4);
    expect(screen.getByText("Database")).toBeTruthy();
  });

  it("redirects legacy /console route to /", async () => {
    (api.me as unknown as ReturnType<typeof vi.fn>).mockResolvedValue({
      email: "admin@example.com",
      orgId: 1,
      isInstanceAdmin: true,
    });
    (api.instanceReadiness as unknown as ReturnType<typeof vi.fn>).mockResolvedValue(FIXABLE_READINESS);

    renderConsole("/console");

    await waitFor(() => {
      expect(screen.getByText("Instance health")).toBeTruthy();
    }, { timeout: 3000 });
    expect(document.querySelectorAll("[data-state]").length).toBe(4);
  });
});

describe("Instance console plugin seam", () => {
  it("shows a plugin rail entry and renders its page when the backend announces it AND the frontend registers it", async () => {
    adminSession();
    (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([INSTANCE_MENU]);
    registerHelloInstancePage();

    renderConsole("/");

    await waitFor(() => {
      expect(screen.getByText("Hello (instance)")).toBeTruthy();
    });

    fireEvent.click(screen.getByText("Hello (instance)"));
    await waitFor(() => {
      expect(screen.getByText("Instance-level page")).toBeTruthy();
    });
  });

  it("hides a backend-announced entry when no frontend route is registered", async () => {
    adminSession();
    (api.instanceMenus as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([INSTANCE_MENU]);

    renderConsole("/");

    await waitFor(() => {
      expect(api.instanceMenus).toHaveBeenCalled();
    });
    expect(screen.queryByText("Hello (instance)")).toBeNull();
  });

  it("hides a frontend-registered entry the backend does not announce", async () => {
    adminSession();
    registerHelloInstancePage();

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
