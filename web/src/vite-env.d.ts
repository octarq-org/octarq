/// <reference types="vite/client" />

interface ImportMetaEnv {
  // Frontend plugin composition flag. Unset in the OSS build (registry stays
  // empty); a commercial build sets `VITE_LED_PLUGINS=pro` to compose Pro UI
  // plugins in. Vite inlines this literal, so the OSS build's registration
  // branch is constant-false and gets dead-code eliminated.
  readonly VITE_LED_PLUGINS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
