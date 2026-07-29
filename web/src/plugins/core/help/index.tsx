import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const HelpViewer = lazy(() => import("./HelpViewer"));

export default {
  name: "help",
  // The `help` namespace backs HelpViewer. It used to be absent entirely — the
  // viewer passed English fallbacks to t(), which renders correctly in every
  // locale and so looks translated while it isn't. Keep the keys here so a
  // missing locale is a visible gap rather than silent English.
  i18n: {
    en: {
      _shared: {
        nav: { help: "Help" },
      },
      title: "Help & Resources",
      loading: "Loading…",
      empty: "No documentation available.",
    },
    zh: {
      _shared: {
        nav: { help: "帮助" },
      },
      title: "帮助与资源",
      loading: "加载中…",
      empty: "暂无可用文档。",
    },
    es: {
      _shared: {
        nav: { help: "Ayuda" },
      },
      title: "Ayuda y recursos",
      loading: "Cargando…",
      empty: "No hay documentación disponible.",
    },
    pt: {
      _shared: {
        nav: { help: "Ajuda" },
      },
      title: "Ajuda e recursos",
      loading: "Carregando…",
      empty: "Nenhuma documentação disponível.",
    },
    ja: {
      _shared: {
        nav: { help: "ヘルプ" },
      },
      title: "ヘルプとリソース",
      loading: "読み込み中…",
      empty: "利用できるドキュメントはありません。",
    },
  },
  routes: [{ path: "/help", Component: HelpViewer }],
} satisfies UIPlugin;
