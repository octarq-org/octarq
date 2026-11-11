import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { fetchOverview, useOverviewData } from "./overview";

describe("overview hook & fetcher", () => {
  const originalFetch = globalThis.fetch;
  let mockTime = 1_000_000;

  beforeEach(() => {
    mockTime += 3000;
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(mockTime);
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  describe("fetchOverview", () => {
    it("calls /api/overview with correct headers and credentials by default", async () => {
      const mockData = { totalClicks: 120, links: 15 };
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockData,
      });
      globalThis.fetch = fetchMock;

      const promise = fetchOverview();
      const data = await promise;

      expect(data).toEqual(mockData);
      expect(fetchMock).toHaveBeenCalledWith("/api/overview", {
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      });
    });

    it("appends ?includeBot=true query when includeBot is true", async () => {
      const mockData = { totalClicks: 200, botClicks7d: 50 };
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockData,
      });
      globalThis.fetch = fetchMock;

      const promise = fetchOverview(true);
      const data = await promise;

      expect(data).toEqual(mockData);
      expect(fetchMock).toHaveBeenCalledWith("/api/overview?includeBot=true", expect.any(Object));
    });

    it("deduplicates concurrent in-flight requests within 2 seconds", async () => {
      const mockData = { links: 5 };
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockData,
      });
      globalThis.fetch = fetchMock;

      const p1 = fetchOverview();
      const p2 = fetchOverview();
      await Promise.all([p1, p2]);

      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it("throws error and purges cache when response is not ok", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
      });
      globalThis.fetch = fetchMock;

      await expect(fetchOverview()).rejects.toThrow("Failed to fetch overview: 500");

      const retryMock = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ links: 1 }),
      });
      globalThis.fetch = retryMock;

      const promise = fetchOverview();
      const data = await promise;
      expect(data).toEqual({ links: 1 });
      expect(retryMock).toHaveBeenCalledTimes(1);
    });
  });

  describe("useOverviewData", () => {
    it("initializes with null and resolves with fetched data", async () => {
      const mockData = { activeLinks: 8, emails: 24 };
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => mockData,
      });

      const { result } = renderHook(() => useOverviewData());
      expect(result.current).toBeNull();

      await waitFor(() => {
        expect(result.current).toEqual(mockData);
      });
    });

    it("handles fetch failure gracefully without throwing", async () => {
      const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
      globalThis.fetch = vi.fn().mockRejectedValue(new Error("Network error"));

      const { result } = renderHook(() => useOverviewData());
      expect(result.current).toBeNull();

      await new Promise((r) => setTimeout(r, 50));
      expect(result.current).toBeNull();
      warnSpy.mockRestore();
    });
  });
});
