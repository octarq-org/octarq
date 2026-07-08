// The example plugin's frontend entry — the JS half's UIPlugin, mirroring the
// Go half's plugin.Plugin in hello.go. A host composes it at build time with
// `registerUIPlugin(helloPlugin)` (see octarq's web/src/plugins/index.ts for the
// commercial-build injection point).
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
    { id: "hello", label: "Hello", path: "/hello", icon: "👋", category: "operations" },
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
