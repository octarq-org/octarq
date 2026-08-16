// Ambient types for `import.meta.env`, the one Vite-ism the SDK surface uses
// (registerUIPlugin's dev/prod split). Vite inlines this at build time in the
// host app; the registry only ever runs inside a Vite-built dashboard. Self-
// contained on purpose — no `/// <reference types="vite/client" />`, because
// vite is not a dependency of this package and pnpm's strict node_modules
// would leave that reference unresolvable.
interface ImportMetaEnv {
  readonly DEV: boolean;
  readonly PROD: boolean;
  readonly MODE: string;
  readonly BASE_URL: string;
  readonly SSR: boolean;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
