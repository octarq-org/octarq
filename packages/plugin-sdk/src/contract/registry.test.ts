import { beforeEach, describe, expect, it } from "vitest";
import type { LazyPage, UIPlugin } from "./types";
import {
  registerUIPlugin,
  resetRegistry,
  uiAreas,
  uiMenus,
  uiPluginI18n,
  uiPluginSharedI18n,
  uiPlugins,
  uiRoutes,
  uiWidgets,
} from "./registry";

// Tests never render these, so a plain marker object is enough. The cast is
// confined to the fixture helper.
const page = (marker: string): LazyPage => ({ marker }) as unknown as LazyPage;

const plugin = (name: string, rest: Partial<UIPlugin> = {}): UIPlugin => ({
  name,
  routes: [],
  ...rest,
});

beforeEach(() => {
  resetRegistry();
});

describe("registerUIPlugin", () => {
  it("registers plugins in order", () => {
    registerUIPlugin(plugin("a"));
    registerUIPlugin(plugin("b"));
    expect(uiPlugins().map((p) => p.name)).toEqual(["a", "b"]);
  });

  it("is idempotent per name: the second registration is ignored", () => {
    const first = plugin("dup", { routes: [{ path: "/first", Component: page("first") }] });
    const second = plugin("dup", { routes: [{ path: "/second", Component: page("second") }] });
    registerUIPlugin(first);
    registerUIPlugin(second);
    expect(uiPlugins()).toHaveLength(1);
    expect(uiPlugins()[0]).toBe(first);
    expect(uiRoutes().map((r) => r.path)).toEqual(["/first"]);
  });
});

describe("uiRoutes", () => {
  it("returns an empty array for an empty registry", () => {
    expect(uiRoutes()).toEqual([]);
  });

  it("flattens routes across plugins in registration order", () => {
    registerUIPlugin(
      plugin("a", {
        routes: [
          { path: "/a1", Component: page("a1") },
          { path: "/a2", Component: page("a2") },
        ],
      }),
    );
    registerUIPlugin(plugin("b", { routes: [{ path: "/b1", Component: page("b1") }] }));
    expect(uiRoutes().map((r) => r.path)).toEqual(["/a1", "/a2", "/b1"]);
  });
});

describe("uiMenus", () => {
  const item = (id: string) => ({ id, label: id, path: `/${id}`, icon: "star", category: "Tools" });

  it("defaults to an empty array (no plugins, or plugins without menus)", () => {
    expect(uiMenus()).toEqual([]);
    registerUIPlugin(plugin("no-menu"));
    expect(uiMenus()).toEqual([]);
  });

  it("flattens menus across plugins, skipping plugins without one", () => {
    registerUIPlugin(plugin("a", { menu: [item("a1"), item("a2")] }));
    registerUIPlugin(plugin("no-menu"));
    registerUIPlugin(plugin("b", { menu: [item("b1")] }));
    expect(uiMenus().map((m) => m.id)).toEqual(["a1", "a2", "b1"]);
  });
});

describe("uiWidgets", () => {
  it("returns an empty array for an unknown slot or empty registry", () => {
    expect(uiWidgets("home-overview")).toEqual([]);
    registerUIPlugin(plugin("a", { widgets: [{ slot: "other", Component: page("x") }] }));
    expect(uiWidgets("home-overview")).toEqual([]);
  });

  it("filters by slot across plugins", () => {
    registerUIPlugin(
      plugin("a", {
        widgets: [
          { slot: "home", Component: page("a-home") },
          { slot: "side", Component: page("a-side") },
        ],
      }),
    );
    registerUIPlugin(plugin("b", { widgets: [{ slot: "home", Component: page("b-home") }] }));
    expect(uiWidgets("home").map((w) => w.Component)).toEqual([page("a-home"), page("b-home")]);
    expect(uiWidgets("side")).toHaveLength(1);
  });

  it("sorts by ascending order, treating missing order as 0", () => {
    registerUIPlugin(
      plugin("a", {
        widgets: [
          { slot: "home", Component: page("third"), order: 5 },
          { slot: "home", Component: page("first"), order: -1 },
          { slot: "home", Component: page("second") }, // implicit 0
        ],
      }),
    );
    expect(uiWidgets("home").map((w) => w.Component)).toEqual([
      page("first"),
      page("second"),
      page("third"),
    ]);
  });

  it("keeps registration order on ties (stable sort)", () => {
    registerUIPlugin(plugin("a", { widgets: [{ slot: "home", Component: page("a"), order: 1 }] }));
    registerUIPlugin(
      plugin("b", {
        widgets: [
          { slot: "home", Component: page("b1"), order: 1 },
          { slot: "home", Component: page("b2") }, // 0 — sorts before all the 1s
        ],
      }),
    );
    registerUIPlugin(plugin("c", { widgets: [{ slot: "home", Component: page("c"), order: 1 }] }));
    expect(uiWidgets("home").map((w) => w.Component)).toEqual([
      page("b2"),
      page("a"),
      page("b1"),
      page("c"),
    ]);
  });
});

describe("uiAreas", () => {
  it("defaults to an empty array", () => {
    expect(uiAreas()).toEqual([]);
    registerUIPlugin(plugin("no-areas"));
    expect(uiAreas()).toEqual([]);
  });

  it("flattens areas across plugins and dedupes by id, first registration wins", () => {
    registerUIPlugin(
      plugin("a", { areas: [{ id: "growth", title: "Growth (a)" }, { id: "ops", title: "Ops" }] }),
    );
    registerUIPlugin(plugin("b", { areas: [{ id: "growth", title: "Growth (b)" }] }));
    const areas = uiAreas();
    expect(areas.map((a) => a.id)).toEqual(["growth", "ops"]);
    expect(areas[0].title).toBe("Growth (a)");
  });
});

describe("uiPluginI18n", () => {
  it("returns empty language maps for an empty registry", () => {
    expect(uiPluginI18n()).toEqual({ en: {}, zh: {} });
  });

  it("merges per-plugin namespaces keyed by plugin name and skips plugins without i18n", () => {
    registerUIPlugin(
      plugin("licenses", { i18n: { en: { title: "Licenses" }, zh: { title: "许可证" } } }),
    );
    registerUIPlugin(plugin("silent"));
    registerUIPlugin(plugin("billing", { i18n: { en: { title: "Billing" }, zh: { title: "账单" } } }));
    expect(uiPluginI18n()).toEqual({
      en: { licenses: { title: "Licenses" }, billing: { title: "Billing" } },
      zh: { licenses: { title: "许可证" }, billing: { title: "账单" } },
    });
  });

  it("excludes _shared from the plugin namespace and deep-merges it via uiPluginSharedI18n, first registration winning", () => {
    registerUIPlugin(
      plugin("ai", {
        i18n: {
          en: { title: "AI", _shared: { settings: { pluginDesc: { ai: "AI things" } } } },
          zh: { title: "AI", _shared: { settings: { pluginDesc: { ai: "AI 功能" } } } },
        },
      }),
    );
    registerUIPlugin(
      plugin("infra", {
        i18n: {
          en: { _shared: { settings: { pluginDesc: { ai: "OVERRIDE", infra: "Servers" } }, nav: { vps: "Servers" } } },
          zh: { _shared: { settings: { pluginDesc: { infra: "服务器" } }, nav: { vps: "服务器" } } },
        },
      }),
    );
    expect(uiPluginI18n().en.ai).toEqual({ title: "AI" });
    expect(uiPluginSharedI18n()).toEqual({
      en: { settings: { pluginDesc: { ai: "AI things", infra: "Servers" } }, nav: { vps: "Servers" } },
      zh: { settings: { pluginDesc: { ai: "AI 功能", infra: "服务器" } }, nav: { vps: "服务器" } },
    });
  });
});

describe("resetRegistry", () => {
  it("clears everything so a name can be re-registered", () => {
    registerUIPlugin(plugin("a", { routes: [{ path: "/a", Component: page("a") }] }));
    resetRegistry();
    expect(uiPlugins()).toEqual([]);
    registerUIPlugin(plugin("a", { routes: [{ path: "/a2", Component: page("a2") }] }));
    expect(uiRoutes().map((r) => r.path)).toEqual(["/a2"]);
  });
});
