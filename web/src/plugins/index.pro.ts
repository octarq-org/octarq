// ─── Frontend plugin injection point (commercial edition) ────────────────────
//
// The commercial counterpart to ./index.ts. A Pro build (VITE_OCTARQ_PLUGINS=pro)
// aliases `#octarq-plugins` to THIS module (see vite.config.ts), so only Pro builds
// import — and therefore only Pro builds bundle — the plugin pages below. The
// OSS build never references this file, so its pages are entirely absent from
// the OSS bundle.
//
// This is where a octarq-pro build lists its plugin set; third-party plugins are
// added the same way, by importing their package and registering it:
//
//   import { helloPlugin } from "@acme/octarq-plugin-hello"; // examples/plugin-hello/web
//   registerUIPlugin(helloPlugin);
import { registerUIPlugin } from "@octarq-org/plugin-sdk";
import { licensesPlugin } from "./licenses";
import { inboxAiPlugin } from "./inbox-ai";
import { llmProvidersPlugin } from "./llm-providers";
import { vpsPlugin } from "./vps";
import { sshKeysPlugin } from "./ssh-keys";
import { financePlugin } from "./finance";
import { storefrontPlugin } from "./storefront";
import { billingPlugin } from "./billing";
import { auditPlugin } from "./audit";

// The Pro plugin set: every former Pro page, now composed through the SDK so its
// route, sidebar entry, and i18n are absent from the OSS build. Order is
// cosmetic — routes/menus are placed by path/category, not registration order.
registerUIPlugin(licensesPlugin);
registerUIPlugin(inboxAiPlugin);
// Route-less: owns the llmProviders.* i18n namespace for the panel embedded in
// the inbox-ai page. No route/menu of its own.
registerUIPlugin(llmProvidersPlugin);
registerUIPlugin(vpsPlugin);
registerUIPlugin(sshKeysPlugin);
registerUIPlugin(financePlugin);
registerUIPlugin(storefrontPlugin);
registerUIPlugin(billingPlugin);
registerUIPlugin(auditPlugin);
