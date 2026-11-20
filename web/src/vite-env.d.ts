/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_OCTARQ_PLUGINS?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
