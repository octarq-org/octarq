import { describe, expect, it } from "vitest";
import { formatUSD } from "./money";

describe("formatUSD", () => {
  it("keeps cents on dollar amounts", () => {
    expect(formatUSD(6)).toBe("$6.00");
    expect(formatUSD(1.5)).toBe("$1.50");
  });

  it("keeps a sub-cent turn visible instead of rounding it to zero", () => {
    expect(formatUSD(0.0096)).toBe("$0.0096");
    expect(formatUSD(0.000045)).toBe("$0.000045");
  });

  it("reads a zero amount with two decimals", () => {
    expect(formatUSD(0)).toBe("$0.00");
  });

  it("does not shorten a sub-dollar amount to one decimal", () => {
    expect(formatUSD(0.5)).toBe("$0.50");
    expect(formatUSD(0.99999)).toBe("$0.99999");
  });
});
