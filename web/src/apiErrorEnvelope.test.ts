import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { req, ApiError } from "./api";

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

  it("formats validation details into error message when present", async () => {
    respondWith({
      code: "validation_failed",
      message: "validation failed",
      details: [
        { location: "body.appName", message: "cannot be empty" },
        { location: "body.dataRetentionDays", message: "expected integer >= 1" },
      ],
    });
    await expect(req("PUT", "/api/instance-settings")).rejects.toThrow(
      "validation failed: appName: cannot be empty, dataRetentionDays: expected integer >= 1",
    );
  });
});
