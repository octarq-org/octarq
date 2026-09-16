// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { I18nProvider } from "../i18n";
import { useNavigation, NAV_CACHE_KEY, clearCachedNav, writeCachedNav } from "./useNavigation";
import { api } from "../api";

vi.mock("../api", () => ({
  api: {
    menus: vi.fn().mockResolvedValue([
      { id: "overview", label: "Overview", path: "/overview", icon: "layout-dashboard", category: "Workspace" },
      { id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" },
      { id: "domains", label: "DNS", path: "/domains", icon: "globe", category: "Network" },
    ]),
    plugins: vi.fn().mockResolvedValue([]),
    actions: vi.fn().mockResolvedValue([]),
    helpCategories: vi.fn().mockResolvedValue([]),
    helpIndex: vi.fn().mockResolvedValue([]),
  },
}));

function wrapper({ children }: { children: React.ReactNode }) {
  return (
    <MemoryRouter initialEntries={["/overview"]}>
      <I18nProvider>{children}</I18nProvider>
    </MemoryRouter>
  );
}

describe("useNavigation hook", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("initializes navigation tree and fetches dynamic backend menus", async () => {
    const { result } = renderHook(
      () => useNavigation({ role: "admin", isInstanceAdmin: true, activeOrgId: 1 }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.backendLoaded).toBe(true);
    });

    expect(result.current.areas.some((a) => a.id === "operations")).toBe(true);
    expect(result.current.areas.some((a) => a.id === "assets")).toBe(true);
    expect(result.current.activeArea).toBe("operations");
    expect(result.current.currentArea.id).toBe("operations");
  });

  it("supports reading and clearing cached navigation data", () => {
    writeCachedNav({
      menus: [{ id: "cached", label: "Cached Item", path: "/cached", icon: "globe", category: "Workspace" }],
      plugins: [],
      actions: [],
    });

    expect(localStorage.getItem(NAV_CACHE_KEY)).toBeTruthy();

    clearCachedNav();
    expect(localStorage.getItem(NAV_CACHE_KEY)).toBeNull();
  });

  it("handles plugin change events by refreshing navigation", async () => {
    const { result } = renderHook(
      () => useNavigation({ role: "admin", isInstanceAdmin: true, activeOrgId: 1 }),
      { wrapper },
    );

    await waitFor(() => {
      expect(result.current.backendLoaded).toBe(true);
    });

    expect(api.menus).toHaveBeenCalledTimes(1);

    act(() => {
      window.dispatchEvent(new Event("octarq:plugins-changed"));
    });

    await waitFor(() => {
      expect(api.menus).toHaveBeenCalledTimes(2);
    });
  });
});
