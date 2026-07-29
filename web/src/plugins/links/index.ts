import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { linksI18n } from "./i18n";

const links: UIPlugin = {
  name: "links",
  routes: [{ path: "/links", Component: lazy(() => import("./pages")) }],
  widgets: [
    { slot: "home-overview", order: 10, Component: lazy(() => import("./widgets/LinksStatCardWidget")) },
    { slot: "home-setup-steps", order: 20, Component: lazy(() => import("./widgets/LinksSetupStepWidget")) },
    { slot: "home-chart", order: 10, Component: lazy(() => import("./widgets/ClicksChartWidget")) },
    { slot: "home-panels", order: 10, Component: lazy(() => import("./widgets/TopLinksPanelWidget")) },
    { slot: "home-panels", order: 20, Component: lazy(() => import("./widgets/TopCitiesPanelWidget")) },
    { slot: "home-panels", order: 30, Component: lazy(() => import("./widgets/DevicesPanelWidget")) },
  ],
  i18n: linksI18n,
};

export default links;

