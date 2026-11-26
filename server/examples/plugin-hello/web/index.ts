import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

export const helloPlugin: UIPlugin = {
  name: "hello",
  routes: [
    { path: "/hello", Component: lazy(() => import("./Page")) },
  ],
  instanceRoutes: [
    { path: "/hello", Component: lazy(() => import("./Page")) },
  ],
  i18n: {
    en: {
      _shared: { nav: { hello: "Hello", "hello-instance": "Hello (instance)" } },
      pageTitle: "Hello Plugin",
      pageDesc: "A minimal full-stack example plugin.",
      feature: "Hello Plugin",
      description: "A minimal example plugin.",
      loading: "Loading…",
    },
    zh: {
      _shared: { nav: { hello: "示例", "hello-instance": "实例示例" } },
      pageTitle: "示例插件",
      pageDesc: "一个最小的全栈示例插件。",
      feature: "示例插件",
      description: "一个最小的示例插件。",
      loading: "加载中…",
    },
    es: {
      _shared: { nav: { hello: "Hola", "hello-instance": "Hola (instancia)" } },
      pageTitle: "Complemento Hola",
      pageDesc: "Un complemento de ejemplo full-stack mínimo.",
      feature: "Complemento Hola",
      description: "Un complemento de ejemplo mínimo.",
      loading: "Cargando…",
    },
    pt: {
      _shared: { nav: { hello: "Olá", "hello-instance": "Olá (instância)" } },
      pageTitle: "Plugin Olá",
      pageDesc: "Um plugin de exemplo full-stack mínimo.",
      feature: "Plugin Olá",
      description: "Um plugin de exemplo mínimo.",
      loading: "Carregando…",
    },
    ja: {
      _shared: { nav: { hello: "ハロー", "hello-instance": "ハロー（インスタンス）" } },
      pageTitle: "Hello プラグイン",
      pageDesc: "最小構成のフルスタック・サンプルプラグイン。",
      feature: "Hello プラグイン",
      description: "最小構成のサンプルプラグイン。",
      loading: "読み込み中…",
    },
  },
};

export default helloPlugin;
