// The AI Inbox UIPlugin — the `ai` backend plugin's inbox page (summaries,
// classification, OTP), composed through the frontend SDK. Its embedded LLM
// providers panel lives in the sibling llm-providers plugin.
//
// Composed only in a commercial build (see ../index.pro.ts). In the OSS build
// this module is never registered, so `/inbox-ai` has no route and no sidebar
// entry — the page bytes are absent, not merely runtime-gated.
import { lazy } from "react";
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { inboxAi } from "./i18n";

export const inboxAiPlugin: UIPlugin = {
  name: "inboxAi",
  routes: [{ path: "/inbox-ai", Component: lazy(() => import("./page")) }],
  // Operations area, matching where the static sidebar used to place AI Inbox.
  menu: [{ id: "inbox-ai", label: "AI Inbox", path: "/inbox-ai", icon: "🤖", category: "Messaging" }],
  i18n: inboxAi,
};
