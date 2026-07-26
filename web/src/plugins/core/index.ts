// Core-feature UIPlugins — the app's own business pages, demoted to plugins.
//
// Truly-core UI (abuse, audit) stays in-tree and always composed here.
// Feature plugins (dns, mail, links) are composed via the plugin manifest
// (octarq.plugins.json).
import { registerUIPlugin } from "@octarq/plugin-sdk";
import abuse from "./abuse";
import audit from "./audit";
import help from "./help";
import notifyCore from "./notify";

for (const p of [abuse, audit, help, notifyCore]) registerUIPlugin(p);

