// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  configurePluginHttp,
  getAppLang,
  pluginURL,
  req,
  resetPluginHttp,
} from "./request";

function jsonResponse(body: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "content-type": "application/json", ...init.headers },
  });
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  resetPluginHttp();
  fetchMock = vi.fn(async () => jsonResponse({ ok: true }));
  vi.stubGlobal("fetch", fetchMock);
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("req", () => {
  it("returns the parsed JSON body", async () => {
    await expect(req<{ ok: boolean }>("GET", "/api/x/thing")).resolves.toEqual({ ok: true });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("sends the saved language and the JSON content type for a body", async () => {
    localStorage.setItem("lang", "ja");
    await req("POST", "/api/x/thing", { name: "a" });
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = init.headers as Record<string, string>;
    expect(headers["Accept-Language"]).toBe("ja");
    expect(headers["Content-Type"]).toBe("application/json");
    expect(init.body).toBe(JSON.stringify({ name: "a" }));
    expect(init.credentials).toBe("same-origin");
  });

  it("sets no content type on a bodyless GET", async () => {
    await req("GET", "/api/x/thing");
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect((init.headers as Record<string, string>)["Content-Type"]).toBeUndefined();
    expect(init.body).toBeUndefined();
  });

  it("lets the caller override the language for one call", async () => {
    localStorage.setItem("lang", "en");
    await req("GET", "/api/x/thing", undefined, "zh");
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect((init.headers as Record<string, string>)["Accept-Language"]).toBe("zh");
  });

  it("folds field details and the request id into one ApiError", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          message: "invalid request",
          details: [{ location: "body.name", message: "must not be empty" }, "too short"],
        }),
        { status: 422, headers: { "content-type": "application/json", "x-request-id": "req-7" } },
      ),
    );
    const error = await req("POST", "/api/x/thing", {}).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    const apiError = error as ApiError;
    expect(apiError.status).toBe(422);
    expect(apiError.message).toBe("invalid request: name: must not be empty, too short");
    expect(apiError.requestId).toBe("req-7");
    expect(apiError.body).toMatchObject({ message: "invalid request" });
  });

  it("falls back to the status text when the error body is not JSON", async () => {
    fetchMock.mockResolvedValueOnce(new Response("gateway down", { status: 502 }));
    const error = (await req("GET", "/api/x/thing").catch((e: unknown) => e)) as ApiError;
    expect(error.status).toBe(502);
    expect(error.message.length).toBeGreaterThan(0);
    expect(error.body).toBeNull();
  });

  it("returns undefined for 204 and for a non-JSON success", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await expect(req("DELETE", "/api/x/thing")).resolves.toBeUndefined();
    fetchMock.mockResolvedValueOnce(new Response("plain", { status: 200, headers: { "content-type": "text/plain" } }));
    await expect(req("GET", "/api/x/thing")).resolves.toBeUndefined();
  });

  it("passes the abort signal through", async () => {
    const controller = new AbortController();
    controller.abort();
    await req("GET", "/api/x/thing", undefined, undefined, controller.signal).catch(() => {});
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.signal).toBe(controller.signal);
  });
});

describe("base path", () => {
  it("prefixes a root-relative path once", () => {
    configurePluginHttp({ basePath: "/api/x/octarq/" });
    expect(pluginURL("/thing")).toBe("/api/x/octarq/thing");
    expect(pluginURL("/api/x/octarq/thing")).toBe("/api/x/octarq/thing");
    expect(pluginURL("https://example.com/thing")).toBe("https://example.com/thing");
  });

  it("sends the prefixed path", async () => {
    configurePluginHttp({ basePath: "/api/x/octarq" });
    await req("GET", "/thing");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/x/octarq/thing");
  });

  it("resetPluginHttp drops the prefix", async () => {
    configurePluginHttp({ basePath: "/api/x/octarq" });
    resetPluginHttp();
    await req("GET", "/thing");
    expect(fetchMock.mock.calls[0][0]).toBe("/thing");
  });
});

describe("getAppLang", () => {
  it("uses the configured language source when present", () => {
    configurePluginHttp({ language: () => "pt" });
    expect(getAppLang()).toBe("pt");
  });

  it("follows the browser when nothing is saved", () => {
    expect(getAppLang()).toMatch(/^(en|zh|es|pt|ja)$/);
  });
});
