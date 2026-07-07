import { defineConfig } from "tsup";

// Two entry points, mirroring the package `exports` map:
//   .    → src/index.ts  (the UIPlugin contract + registry)
//   ./ui → src/ui/index.ts (the shared glass-themed component surface)
// ESM only, with generated .d.ts. React / React DOM stay external (peers).
export default defineConfig({
  entry: {
    index: "src/index.ts",
    ui: "src/ui/index.ts",
  },
  format: ["esm"],
  dts: true,
  clean: true,
  sourcemap: true,
  treeshake: true,
  external: ["react", "react-dom"],
});
