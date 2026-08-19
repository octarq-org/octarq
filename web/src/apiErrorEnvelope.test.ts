import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { req, ApiError } from "./api";

// The server answers errors with the octarq envelope (internal/apierror):
// {code, message, details, request_id}. It replaced huma's RFC 7807 document,
// whose prose lived in `detail`, and an older ad-hoc {error} shape.
//
// This exists because the backend half of that change shipped without the
// frontend half. api.ts still read `error` then `detail`, found neither, and
// fell back to res.statusText — so every error surface in the dashboard showed
// "Unauthorized" where it used to show what actually went wrong. The only thing
// that caught it was one e2e assertion on the login screen; nothing here failed.
describe("API error envelope", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const respondWith = (body: unknown) => {
    (globalThis.fetch as any).mockResolvedValue({
      ok: false,
      status: 401,
      statusText: "Unauthorized",
      headers: { get: () => null },
      json: async () => body,
    });
  };

  it("surfaces `message` from the octarq envelope", async () => {
    respondWith({ code: "unauthorized", message: "invalid credentials" });
    await expect(req("POST", "/api/auth/login")).rejects.toThrow("invalid credentials");
  });

  it("still reads the pre-envelope `detail` and `error` shapes", async () => {
    respondWith({ detail: "invalid credentials" });
    await expect(req("POST", "/api/auth/login")).rejects.toThrow("invalid credentials");

    respondWith({ error: "invalid credentials" });
    await expect(req("POST", "/api/auth/login")).rejects.toThrow("invalid credentials");
  });

  it("falls back to statusText only when the body carries no message", async () => {
    respondWith({ code: "unauthorized" });
    await expect(req("POST", "/api/auth/login")).rejects.toThrow("Unauthorized");
  });

  it("keeps the status on the thrown ApiError", async () => {
    respondWith({ code: "unauthorized", message: "invalid credentials" });
    await expect(req("POST", "/api/auth/login")).rejects.toBeInstanceOf(ApiError);
  });
});
