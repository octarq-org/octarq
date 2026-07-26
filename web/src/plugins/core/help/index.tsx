import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const HelpViewer = lazy(() => import("./HelpViewer"));

export default {
  name: "help",
  i18n: {
    en: {
      _shared: {
        nav: { help: "Help" },
        groups: {
          "Help & Resources": "Help & Resources",
        },
      },
    },
    zh: {
      _shared: {
        nav: { help: "帮助" },
        groups: {
          "Help & Resources": "帮助与资源",
        },
      },
    },
    es: {
      _shared: {
        nav: { help: "Ayuda" },
        groups: {
          "Help & Resources": "Ayuda y Recursos",
        },
      },
    },
    pt: {
      _shared: {
        nav: { help: "Ajuda" },
        groups: {
          "Help & Resources": "Ajuda e Recursos",
        },
      },
    },
    ja: {
      _shared: {
        nav: { help: "ヘルプ" },
        groups: {
          "Help & Resources": "ヘルプとリソース",
        },
      },
    },
  },
  routes: [{ path: "/help", Component: HelpViewer }],
} satisfies UIPlugin;
