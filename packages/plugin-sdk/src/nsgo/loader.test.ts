// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { compileApp, forgetApp, loadManifest, loadLoader, NsgoLoadError } from "./loader";

const manifest = { wasm: "app.wasm", exec: "wasm_exec.js", loader: "nsgo.js" };

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async () =>
    new Response(JSON.stringify(manifest), {
      status: 200,
      headers: { "content-type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("loadManifest", () => {
  it("fetches one manifest per app and resolves its URLs against it", async () => {
    const first = await loadManifest("/console/nsgo.json");
    const second = await loadManifest("/console/nsgo.json");
    expect(second).toBe(first);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(first.base).toBe(new URL("/console/nsgo.json", document.baseURI).href);
    forgetApp("/console/nsgo.json");
  });

  it("reports the HTTP status of a missing build", async () => {
    fetchMock.mockResolvedValueOnce(new Response("not found", { status: 404 }));
    const error = (await loadManifest("/missing/nsgo.json").catch((e: unknown) => e)) as NsgoLoadError;
    expect(error).toBeInstanceOf(NsgoLoadError);
    expect(error.status).toBe(404);
  });

  it("does not cache a failure, so a retry after an install succeeds", async () => {
    fetchMock.mockResolvedValueOnce(new Response("gone", { status: 503 }));
    await loadManifest("/late/nsgo.json").catch(() => {});
    const second = await loadManifest("/late/nsgo.json");
    expect(second.manifest.wasm).toBe("app.wasm");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    forgetApp("/late/nsgo.json");
  });

  it("refuses a manifest that names no files", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ wasm: "a.wasm" }), { status: 200 }));
    const error = (await loadManifest("/broken/nsgo.json").catch((e: unknown) => e)) as NsgoLoadError;
    expect(error).toBeInstanceOf(NsgoLoadError);
    expect(error.message).toContain("names no wasm, exec or loader");
  });
});

describe("compileApp", () => {
  it("compiles one module per app", async () => {
    const compiled = { fake: "module" } as unknown as WebAssembly.Module;
    const compile = vi.spyOn(WebAssembly, "compileStreaming").mockResolvedValue(compiled);
    const first = await compileApp("/console/app.wasm");
    const second = await compileApp("/console/app.wasm");
    expect(first).toBe(compiled);
    expect(second).toBe(compiled);
    expect(compile).toHaveBeenCalledTimes(1);
  });

  it("reports a missing binary with its status", async () => {
    fetchMock.mockResolvedValueOnce(new Response("gone", { status: 404 }));
    const error = (await compileApp("/console/gone.wasm").catch((e: unknown) => e)) as NsgoLoadError;
    expect(error.status).toBe(404);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("loadLoader", () => {
  it("shares one import while it is in flight and retries after it fails", async () => {
    const first = loadLoader("/definitely-missing/nsgo.js");
    const during = loadLoader("/definitely-missing/nsgo.js");
    expect(during).toBe(first);
    await expect(first).rejects.toBeTruthy();
    const after = loadLoader("/definitely-missing/nsgo.js");
    expect(after).not.toBe(first);
    await expect(after).rejects.toBeTruthy();
  });
});
