// ─── Frontend plugin injection point (commercial edition) ────────────────────
//
// The commercial counterpart to ./index.ts. A Pro build (VITE_LED_PLUGINS=pro)
// aliases `#led-plugins` to THIS module (see vite.config.ts), so only Pro builds
// import — and therefore only Pro builds bundle — the plugin pages below. The
// OSS build never references this file, so its pages are entirely absent from
// the OSS bundle.
//
// This is where a led-pro build lists its plugin set; third-party plugins are
// added the same way, by importing their package and registering it:
//
//   import { helloPlugin } from "@acme/led-plugin-hello"; // examples/plugin-hello/web
//   registerUIPlugin(helloPlugin);
import { registerUIPlugin } from "@led/plugin-sdk";
import { licensesPlugin } from "./licenses";

// The licenses PoC: the existing Pro page, now composed through the SDK.
registerUIPlugin(licensesPlugin);
