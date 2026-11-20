// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, render } from "@testing-library/react";
import { setTheme } from "../../theme";
import { CATALOG } from "./catalog";
import { Inspector } from "./Inspector";

describe("Inspector token table", () => {
  afterEach(() => setTheme("light"));

  it("reads the token table a bounded number of times per theme flip", () => {
    const computed = vi.spyOn(window, "getComputedStyle");
    render(<Inspector entry={CATALOG[0]} />);
    const afterMount = computed.mock.calls.length;

    act(() => setTheme("dark"));
    act(() => setTheme("light"));

    expect(computed.mock.calls.length).toBeLessThanOrEqual(afterMount + 4);
    computed.mockRestore();
  });
});
