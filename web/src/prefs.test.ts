// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { setShowPersonalArea, useShowPersonalArea } from "./prefs";

const KEY = "octarq-show-personal";

describe("showPersonalArea preference", () => {
  beforeEach(() => {
    localStorage.clear();
    setShowPersonalArea(true);
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("defaults to true when nothing is stored", () => {
    const { result } = renderHook(() => useShowPersonalArea());
    expect(result.current).toBe(true);
  });

  it("round-trips through storage", () => {
    const { result } = renderHook(() => useShowPersonalArea());

    act(() => setShowPersonalArea(false));
    expect(result.current).toBe(false);
    expect(localStorage.getItem(KEY)).toBe("false");

    act(() => setShowPersonalArea(true));
    expect(result.current).toBe(true);
    expect(localStorage.getItem(KEY)).toBe("true");
  });

  it("keeps the in-memory value when storage throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage disabled");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage disabled");
    });

    const { result } = renderHook(() => useShowPersonalArea());
    expect(result.current).toBe(true);

    act(() => setShowPersonalArea(false));
    expect(result.current).toBe(false);

    act(() => setShowPersonalArea(true));
    expect(result.current).toBe(true);
  });
});
