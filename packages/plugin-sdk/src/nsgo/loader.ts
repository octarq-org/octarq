export interface NsgoManifest {
  wasm: string;
  exec: string;
  loader: string;
  host?: string;
  resources?: unknown;
  runtime?: string;
  scripts?: string[];
}

export type NsgoAppearance = "light" | "dark" | "system";

export interface NsgoPlayer {
  hostEvent(name: string, payload?: unknown): void;
  setAppearance(value: NsgoAppearance): void;
  setLocale(tag: string): void;
  setFontScale(scale: number): void;
}

export interface NsgoHostHandlers {
  call?: (name: string, args: unknown) => unknown;
  post?: (name: string, args: unknown) => void;
  openURL?: (url: string) => boolean | Promise<boolean>;
}

export interface NsgoMountOptions {
  params?: Record<string, unknown>;
  appearance?: NsgoAppearance;
  locale?: string;
  fontScale?: number;
  host?: NsgoHostHandlers;
  config?: NsgoManifest;
}

export interface NsgoLoader {
  mount(
    element: HTMLElement | null,
    url: string | null,
    options?: NsgoMountOptions & { module?: WebAssembly.Module },
  ): Promise<NsgoPlayer>;
}

export class NsgoLoadError extends Error {
  url: string;
  status?: number;
  constructor(url: string, message: string, status?: number) {
    super(message);
    this.name = "NsgoLoadError";
    this.url = url;
    this.status = status;
  }
}

const manifests = new Map<string, Promise<{ manifest: NsgoManifest; base: string }>>();
const loaders = new Map<string, Promise<NsgoLoader>>();
const modules = new Map<string, Promise<WebAssembly.Module>>();

function absolute(name: string, base: string): string {
  return new URL(name, base).href;
}

async function fetchManifest(src: string) {
  const base = new URL(src, document.baseURI).href;
  const res = await fetch(base, { credentials: "same-origin" });
  if (!res.ok) {
    throw new NsgoLoadError(src, `The workbench manifest returned HTTP ${res.status}.`, res.status);
  }
  const manifest = (await res.json()) as NsgoManifest;
  if (!manifest.wasm || !manifest.exec || !manifest.loader) {
    throw new NsgoLoadError(src, "The workbench manifest names no wasm, exec or loader file.");
  }
  return { manifest, base };
}

export function loadManifest(src: string) {
  let hit = manifests.get(src);
  if (!hit) {
    hit = fetchManifest(src);
    manifests.set(src, hit);
    hit.catch(() => manifests.delete(src));
  }
  return hit;
}

export function loadLoader(loaderURL: string): Promise<NsgoLoader> {
  let hit = loaders.get(loaderURL);
  if (!hit) {
    hit = import(/* @vite-ignore */ loaderURL).then((mod: Partial<NsgoLoader>) => {
      if (typeof mod.mount !== "function") {
        throw new NsgoLoadError(loaderURL, "The workbench loader exports no mount function.");
      }
      return mod as NsgoLoader;
    });
    loaders.set(loaderURL, hit);
    hit.catch(() => loaders.delete(loaderURL));
  }
  return hit;
}

export function compileApp(wasmURL: string): Promise<WebAssembly.Module> {
  let hit = modules.get(wasmURL);
  if (!hit) {
    hit = (async () => {
      const res = await fetch(wasmURL, { credentials: "same-origin" });
      if (!res.ok) {
        throw new NsgoLoadError(wasmURL, `The workbench binary returned HTTP ${res.status}.`, res.status);
      }
      return WebAssembly.compileStreaming(res);
    })();
    modules.set(wasmURL, hit);
    hit.catch(() => modules.delete(wasmURL));
  }
  return hit;
}

export interface LoadedApp {
  loader: NsgoLoader;
  manifest: NsgoManifest;
  base: string;
  module: WebAssembly.Module;
}

export async function loadApp(src: string): Promise<LoadedApp> {
  const { manifest, base } = await loadManifest(src);
  const [loader, module] = await Promise.all([
    loadLoader(absolute(manifest.loader, base)),
    compileApp(absolute(manifest.wasm, base)),
  ]);
  return { loader, manifest, base, module };
}

export function forgetApp(src: string): void {
  const hit = manifests.get(src);
  manifests.delete(src);
  void hit?.then(({ manifest, base }) => {
    loaders.delete(absolute(manifest.loader, base));
    modules.delete(absolute(manifest.wasm, base));
  });
}
