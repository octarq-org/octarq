import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const HelpViewer = lazy(() => import("./HelpViewer"));

export default {
  name: "help",
  i18n: {
    en: {
      _shared: {
        groups: {
          "Help & Resources": "Help & Resources",
        },
      },
    },
    zh: {
      _shared: {
        groups: {
          "Help & Resources": "帮助与资源",
        },
      },
    },
    es: {
      _shared: {
        groups: {
          "Help & Resources": "Ayuda y Recursos",
        },
      },
    },
    pt: {
      _shared: {
        groups: {
          "Help & Resources": "Ajuda e Recursos",
        },
      },
    },
    ja: {
      _shared: {
        groups: {
          "Help & Resources": "ヘルプとリソース",
        },
      },
    },
  },
  routes: [{ path: "/help", Component: HelpViewer }],
} satisfies UIPlugin;
