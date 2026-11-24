import { registerUIPlugin } from "@octarq/plugin-sdk";
import audit from "./audit";
import help from "./help";
import notifyCore from "./notify";

for (const p of [audit, help, notifyCore]) registerUIPlugin(p);

