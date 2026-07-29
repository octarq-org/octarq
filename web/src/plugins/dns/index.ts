import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { domainsI18n } from "./i18n";

const domains: UIPlugin = {
  name: "domains",
  routes: [{ path: "/domains", Component: lazy(() => import("./pages")) }],
  widgets: [
    { slot: "home-overview", order: 30, Component: lazy(() => import("./widgets/DNSStatCardWidget")) },
    { slot: "home-setup-steps", order: 10, Component: lazy(() => import("./widgets/DNSSetupStepWidget")) },
  ],
  i18n: domainsI18n,
};

export default domains;

