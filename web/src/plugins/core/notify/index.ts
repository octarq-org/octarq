import { lazy } from "react";
import type { UIPlugin } from "@octarq/plugin-sdk";

const notifyCore: UIPlugin = {
  name: "notify-core",
  routes: [],
  widgets: [
    { slot: "settings-notification-channel:telegram", Component: lazy(() => import("./TelegramForm")) },
    { slot: "settings-notification-channel:webhook", Component: lazy(() => import("./WebhookForm")) },
    { slot: "settings-notification-channel:email", Component: lazy(() => import("./EmailForm")) },
  ],
};

export default notifyCore;
