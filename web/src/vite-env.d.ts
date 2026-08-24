/// <reference types="vite/client" />

interface ImportMetaEnv {
  // Frontend plugin composition flag. Unset in the OSS build (registry stays
  // empty); a downstream build sets `VITE_OCTARQ_PLUGINS=...` to compose UI
  // plugins in. Vite inlines this literal, so the OSS build's registration
  // branch is constant-false and gets dead-code eliminated.
  readonly VITE_OCTARQ_PLUGINS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
