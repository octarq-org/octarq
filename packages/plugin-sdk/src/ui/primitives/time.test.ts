import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { timeAgo } from "./time";

describe("timeAgo", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-01-01T12:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("steps through seconds, minutes, hours and days", () => {
    expect(timeAgo("2026-01-01T11:59:30Z")).toBe("30s ago");
    expect(timeAgo("2026-01-01T11:45:00Z")).toBe("15m ago");
    expect(timeAgo("2026-01-01T09:00:00Z")).toBe("3h ago");
    expect(timeAgo("2025-12-29T12:00:00Z")).toBe("3d ago");
  });

  it("changes unit exactly at each boundary, first unit to cross winning", () => {
    expect(timeAgo("2026-01-01T11:59:01Z")).toBe("59s ago");
    expect(timeAgo("2026-01-01T11:59:00Z")).toBe("1m ago");
    expect(timeAgo("2026-01-01T11:00:01Z")).toBe("59m ago");
    expect(timeAgo("2026-01-01T11:00:00Z")).toBe("1h ago");
    expect(timeAgo("2025-12-31T12:00:01Z")).toBe("23h ago");
    expect(timeAgo("2025-12-31T12:00:00Z")).toBe("1d ago");
  });
});
