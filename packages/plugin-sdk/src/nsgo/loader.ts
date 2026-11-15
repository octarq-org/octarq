// The loader cache behind <NsgoMount />.
//
// An nsgo web app is a directory `nsgo build -target web` writes: `nsgo.json`
// names the app's WebAssembly, the Go runtime script and the loader, relative
// to itself. The loader (`nsgo.js`) exports `mount(element, url, options)`.
//
// This module fetches each app's manifest once, imports its loader once, and
// compiles its WebAssembly once, no matter how many mounts use it — compiling
// 35 MB per mount would make a route switch feel like a cold start.
export interface NsgoManifest {
  wasm: string;
  exec: string;
  loader: string;
  // A host module the app may name instead of taking one from the page.
  host?: string;
  resources?: unknown;
  runtime?: string;
  scripts?: string[];
}

export type NsgoAppearance = "light" | "dark" | "system";

// The player the loader resolves with. Methods take effect immediately.
export interface NsgoPlayer {
  hostEvent(name: string, payload?: unknown): void;
  setAppearance(value: NsgoAppearance): void;
  setLocale(tag: string): void;
  setFontScale(scale: number): void;
}

// The host's answers to the app. `call` returns (or resolves with) the result;
// a throw or rejection fails the call with its message. `post` receives events
// that need no answer. `openURL` returns whether the host took the URL.
export interface NsgoHostHandlers {
  call?: (name: string, args: unknown) => unknown;
  post?: (name: string, args: unknown) => void;
  openURL?: (url: string) => boolean | Promise<boolean>;
}

export interface NsgoMountOptions {
  // Startup parameters. nsgo fixes them for the app's life; later messages are
  // events, not parameter changes.
  params?: Record<string, unknown>;
  appearance?: NsgoAppearance;
  locale?: string;
  fontScale?: number;
  host?: NsgoHostHandlers;
  // The manifest content when the caller already has it.
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
  // The HTTP status when the failure was a response; undefined otherwise.
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

// loadManifest reads the app's nsgo.json. One fetch per app per page.
export function loadManifest(src: string) {
  let hit = manifests.get(src);
  if (!hit) {
    hit = fetchManifest(src);
    manifests.set(src, hit);
    // A failed load is not cached: a retry after the operator installs the
    // build must succeed without a page reload.
    hit.catch(() => manifests.delete(src));
  }
  return hit;
}

// loadLoader imports the app's loader module once.
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

// compileApp compiles the app's WebAssembly once and shares the module.
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

// loadApp resolves everything a mount needs. Every step is cached, so the
// second mount of the same app costs one `mount` call.
export async function loadApp(src: string): Promise<LoadedApp> {
  const { manifest, base } = await loadManifest(src);
  const [loader, module] = await Promise.all([
    loadLoader(absolute(manifest.loader, base)),
    compileApp(absolute(manifest.wasm, base)),
  ]);
  return { loader, manifest, base, module };
}

// forgetApp drops every cached step for one app. The test suite uses it; a
// host calls it after replacing the build on disk.
export function forgetApp(src: string): void {
  const hit = manifests.get(src);
  manifests.delete(src);
  void hit?.then(({ manifest, base }) => {
    loaders.delete(absolute(manifest.loader, base));
    modules.delete(absolute(manifest.wasm, base));
  });
}
