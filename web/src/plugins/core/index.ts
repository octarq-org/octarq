// Core-feature UIPlugins — the app's own business pages, demoted to plugins.
//
// Truly-core UI (abuse, audit, assets) stays in-tree and always composed here.
// Feature plugins (dns, mail, links) are composed via the plugin manifest
// (octarq.plugins.json).
import { registerUIPlugin } from "@octarq-org/plugin-sdk";
import assets from "./assets";
import abuse from "./abuse";
import audit from "./audit";

for (const p of [assets, abuse, audit]) registerUIPlugin(p);
