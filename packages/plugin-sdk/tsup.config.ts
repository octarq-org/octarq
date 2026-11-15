import { defineConfig } from "tsup";

// Three entry points, mirroring the package `exports` map:
//   .         → src/index.ts      (the UIPlugin contract + registry + data)
//   ./ui      → src/ui/index.ts   (the shared glass-themed component surface)
//   ./testing → src/testing/index.tsx (the plugin test harness)
// ESM only, with generated .d.ts. React, React DOM and react-query stay
// external (peers).
export default defineConfig({
  entry: {
    index: "src/index.ts",
    ui: "src/ui/index.ts",
    testing: "src/testing/index.tsx",
  },
  format: ["esm", "cjs"],
  dts: true,
  clean: true,
  sourcemap: true,
  treeshake: true,
  external: ["react", "react-dom", "@tanstack/react-query"],
});
