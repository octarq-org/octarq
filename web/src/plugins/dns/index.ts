import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { domainsI18n } from "./i18n";

const domains: UIPlugin = {
  name: "domains",
  routes: [{ path: "/domains", Component: lazy(() => import("./pages")) }],
  menu: [{ id: "domains", label: "DNS", path: "/domains", icon: "globe", category: "Network" }],
  i18n: domainsI18n,
};

export default domains;
