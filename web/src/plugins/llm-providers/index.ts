// The LLM Providers UIPlugin — a ROUTE-LESS plugin. LLMProviders isn't a
// standalone page: it is embedded inside the inbox-ai page (and self-gates with
// 402). This plugin carries no route and no sidebar entry; it exists so the
// embedded component's `llmProviders.*` i18n namespace is composed alongside the
// other Pro plugins (a plugin owns exactly one i18n namespace, keyed by `name`).
//
// The component itself is imported directly by inbox-ai's page (./page here).
import type { UIPlugin } from "@octarq-org/plugin-sdk";
import { llmProviders } from "./i18n";

export const llmProvidersPlugin: UIPlugin = {
  name: "llmProviders",
  routes: [],
  i18n: llmProviders,
};
