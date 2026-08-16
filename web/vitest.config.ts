import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

// Merged with the app's vite config rather than standing alone. Without the
// merge, `resolve.alias` doesn't apply under vitest, so `@octarq/plugin-sdk`
// resolves to the workspace package instead of the app-side facade the alias
// points at — a different module with a different export surface. Tests then
// import a real module that silently lacks half its exports (every symbol reads
// as undefined), which is how brandGlyph.test.ts came to assert against
// `undefined` while the app itself rendered the glyph fine.
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      include: ["src/**/*.test.{ts,tsx}"],
      // Global network guard: any test that reaches real fetch() fails with a
      // method+URL error instead of silently hitting the network (see
      // src/test/setup.ts and src/test/networkGuard.test.ts).
      setupFiles: ["./src/test/setup.ts"],
    },
  }),
);
