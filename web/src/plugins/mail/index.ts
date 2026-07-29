import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";
import { mailI18n } from "./i18n";

const mail: UIPlugin = {
  name: "mail",
  routes: [{ path: "/mail", Component: lazy(() => import("./pages")) }],
  widgets: [
    { slot: "home-overview", order: 20, Component: lazy(() => import("./widgets/MailStatCardWidget")) },
    { slot: "home-setup-steps", order: 30, Component: lazy(() => import("./widgets/MailSetupStepWidget")) },
    { slot: "home-rows", order: 10, Component: lazy(() => import("./widgets/RecentMailPanelWidget")) },
  ],
  i18n: mailI18n,
};

export default mail;
