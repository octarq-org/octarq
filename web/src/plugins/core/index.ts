// Core-feature UIPlugins — the app's own business pages, demoted to plugins.
//
// The shell (App.tsx) owns only auth, settings, org handling, Overview and the
// plugin pipeline; EVERY business page — core or Pro — is a UIPlugin composed
// through the same registry (`registerUIPlugin` → uiPlugins()/uiMenus()). Core
// plugins differ from manifest plugins in exactly one way: they are trusted,
// in-tree and always composed (imported unconditionally from main.tsx, before
// the `#octarq-plugins` manifest module), so no edition manifest can forget
// them and their menu entries precede Pro entries within a shared group.
//
// Registration order is menu order within a group (e.g. "Network" lists
// domains before certs because domains registers first).
import { registerUIPlugin } from "@octarq-org/plugin-sdk";
import links from "./links";
import mail from "./mail";
import domains from "./domains";
import assets from "./assets";
import abuse from "./abuse";
import audit from "./audit";

for (const p of [links, mail, domains, assets, abuse, audit]) registerUIPlugin(p);
