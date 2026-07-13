// The example plugin's frontend entry — the JS half's UIPlugin, mirroring the
// Go half's plugin.Plugin in hello.go. A host composes it at build time with
// `registerUIPlugin(helloPlugin)` — in octarq that call is generated into the
// `#octarq-plugins` module from the plugin manifest (web/octarq.plugins.json,
// see web/plugins-manifest.ts). Besides routes/menu/i18n a UIPlugin may also
// contribute dashboard `widgets` (rendered by <ExtensionSlot>, e.g. slot
// "home-overview") and NEW top-level sidebar `areas` — see the UIPlugin type.
//
// In a real distribution this file is the `main`/`exports` of a published npm
// package (e.g. `@acme/octarq-plugin-hello`) that depends on `@octarq-org/plugin-sdk` as a
// peer; the host imports it by name instead of by relative path.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [
    { path: "/hello", Component: lazy(() => import("./Page")) },
  ],
  menu: [
    // `category` names the sidebar GROUP the entry joins: it must equal the
    // group's label (here the "Workspace" group next to Overview). A category
    // with no matching group creates one, and areaForCategory's keyword
    // routing picks the top-level area. Keep it in sync with hello.go Menus().
    { id: "hello", label: "Hello", path: "/hello", icon: "👋", category: "Workspace" },
  ],
  i18n: {
    en: {
      pageTitle: "Hello Plugin",
      pageDesc: "A minimal full-stack example plugin.",
      feature: "Hello Plugin",
      description: "A minimal example plugin.",
      loading: "Loading…",
    },
    zh: {
      pageTitle: "示例插件",
      pageDesc: "一个最小的全栈示例插件。",
      feature: "示例插件",
      description: "一个最小的示例插件。",
      loading: "加载中…",
    },
  },
};

// A plugin package default-exports its UIPlugin (or an array of them), so the
// manifest can compose it with `import helloPlugin from "@acme/octarq-plugin-hello"`.
export default helloPlugin;
