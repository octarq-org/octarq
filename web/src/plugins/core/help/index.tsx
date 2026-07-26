import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const HelpViewer = lazy(() => import("./HelpViewer"));

export default {
  name: "help",
  i18n: {
    en: {
      _shared: {
        nav: { help: "Help" },
      },
    },
    zh: {
      _shared: {
        nav: { help: "帮助" },
      },
    },
    es: {
      _shared: {
        nav: { help: "Ayuda" },
      },
    },
    pt: {
      _shared: {
        nav: { help: "Ajuda" },
      },
    },
    ja: {
      _shared: {
        nav: { help: "ヘルプ" },
      },
    },
  },
  routes: [{ path: "/help", Component: HelpViewer }],
} satisfies UIPlugin;
