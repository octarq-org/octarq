// Domains/DNS — core feature as a UIPlugin (see ./index.ts for the convention).
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";

const domains: UIPlugin = {
  name: "domains",
  routes: [{ path: "/domains", Component: lazy(() => import("../../pages/Domains")) }],
  menu: [{ id: "domains", label: "DNS", path: "/domains", icon: "globe", category: "Network" }],
};

export default domains;
