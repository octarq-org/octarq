import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { mailI18n } from "./i18n";

const mail: UIPlugin = {
  name: "mail",
  routes: [{ path: "/mail", Component: lazy(() => import("./pages")) }],
  menu: [{ id: "mail", label: "Mail", path: "/mail", icon: "mail", category: "Messaging" }],
  i18n: mailI18n,
};

export default mail;
