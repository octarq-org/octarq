// The example plugin's frontend entry — the JS half's UIPlugin, mirroring the
// Go half's plugin.Plugin in hello.go. A host composes it at build time with
// `registerUIPlugin(helloPlugin)` — in octarq that call is generated into the
// `#octarq-plugins` module from the plugin manifest (web/octarq.plugins.json,
// see web/plugins-manifest.ts). Besides routes/menu/i18n a UIPlugin may also
// contribute dashboard `widgets` (rendered by <ExtensionSlot>, e.g. slot
// "home-overview") and NEW top-level sidebar `areas` — see the UIPlugin type.
//
// In a real distribution this file is the `main`/`exports` of a published npm
// package (e.g. `@acme/octarq-plugin-hello`) that depends on `@octarq/plugin-sdk` as a
// peer; the host imports it by name instead of by relative path.
import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [
    { path: "/hello", Component: lazy(() => import("./Page")) },
  ],
  // Sidebar placement comes from the Go half's Menus() (hello.go) alone.
  //
  // Shell renders each entry via `nav.<id>` key; falls back to English label from Go source.
  i18n: {
    en: {
      _shared: { nav: { hello: "Hello" } },
      pageTitle: "Hello Plugin",
      pageDesc: "A minimal full-stack example plugin.",
      feature: "Hello Plugin",
      description: "A minimal example plugin.",
      loading: "Loading…",
    },
    zh: {
      _shared: { nav: { hello: "示例" } },
      pageTitle: "示例插件",
      pageDesc: "一个最小的全栈示例插件。",
      feature: "示例插件",
      description: "一个最小的示例插件。",
      loading: "加载中…",
    },
    es: {
      _shared: { nav: { hello: "Hola" } },
      pageTitle: "Complemento Hola",
      pageDesc: "Un complemento de ejemplo full-stack mínimo.",
      feature: "Complemento Hola",
      description: "Un complemento de ejemplo mínimo.",
      loading: "Cargando…",
    },
    pt: {
      _shared: { nav: { hello: "Olá" } },
      pageTitle: "Plugin Olá",
      pageDesc: "Um plugin de exemplo full-stack mínimo.",
      feature: "Plugin Olá",
      description: "Um plugin de exemplo mínimo.",
      loading: "Carregando…",
    },
    ja: {
      _shared: { nav: { hello: "ハロー" } },
      pageTitle: "Hello プラグイン",
      pageDesc: "最小構成のフルスタック・サンプルプラグイン。",
      feature: "Hello プラグイン",
      description: "最小構成のサンプルプラグイン。",
      loading: "読み込み中…",
    },
  },
};

// A plugin package default-exports its UIPlugin (or an array of them), so the
// manifest can compose it with `import helloPlugin from "@acme/octarq-plugin-hello"`.
export default helloPlugin;
