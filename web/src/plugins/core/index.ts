import { registerUIPlugin } from "@octarq/plugin-sdk";
import abuse from "./abuse";
import audit from "./audit";
import help from "./help";
import notifyCore from "./notify";

for (const p of [abuse, audit, help, notifyCore]) registerUIPlugin(p);

