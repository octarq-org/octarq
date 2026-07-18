import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { linksI18n } from "./i18n";

const links: UIPlugin = {
  name: "links",
  routes: [{ path: "/links", Component: lazy(() => import("./pages")) }],
  menu: [{ id: "links", label: "Links", path: "/links", icon: "link-2", category: "Marketing" }],
  i18n: linksI18n,
};

export default links;
