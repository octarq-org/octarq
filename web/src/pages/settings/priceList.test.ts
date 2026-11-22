import { describe, expect, it } from "vitest";
import type { ModelPriceDTO } from "../../api/types";
import { filterModelPrices, formatPrice, modelId, paginatePrices } from "./priceList";

const row = (model: string, providerId: string): ModelPriceDTO => ({
  model,
  displayName: model.toUpperCase(),
  providerId,
  input: 1,
  output: 2,
  reasoning: 2,
});

const rows = [
  row("qwen-flash", "alibaba-qwen"),
  row("gpt-4o", "openai"),
  row("claude-sonnet", "anthropic"),
];

describe("filterModelPrices", () => {
  it("returns every row for an empty filter", () => {
    expect(filterModelPrices(rows, "  ", "all")).toHaveLength(3);
  });

  it("matches model, display name and provider", () => {
    expect(filterModelPrices(rows, "QWEN", "all").map((r) => r.model)).toEqual(["qwen-flash"]);
    expect(filterModelPrices(rows, "claude", "all").map((r) => r.model)).toEqual(["claude-sonnet"]);
    expect(filterModelPrices(rows, "openai", "all").map((r) => r.model)).toEqual(["gpt-4o"]);
  });

  it("narrows by provider", () => {
    expect(filterModelPrices(rows, "", "openai").map((r) => r.model)).toEqual(["gpt-4o"]);
    expect(filterModelPrices(rows, "qwen", "openai")).toEqual([]);
  });
});

describe("paginatePrices", () => {
  const many = Array.from({ length: 25 }, (_, i) => row(`model-${i + 1}`, "openai"));

  it("slices the requested page", () => {
    const first = paginatePrices(many, 1, 20);
    expect(first.page).toBe(1);
    expect(first.pageCount).toBe(2);
    expect(first.rows).toHaveLength(20);

    const second = paginatePrices(many, 2, 20);
    expect(second.rows.map((r) => r.model)).toEqual(["model-21", "model-22", "model-23", "model-24", "model-25"]);
  });

  it("clamps a page past the end", () => {
    const page = paginatePrices(many, 9, 20);
    expect(page.page).toBe(2);
    expect(page.rows[0].model).toBe("model-21");
  });

  it("keeps at least one page when the list is empty", () => {
    const page = paginatePrices([], 3, 20);
    expect(page.pageCount).toBe(1);
    expect(page.page).toBe(1);
    expect(page.rows).toEqual([]);
  });
});

describe("modelId", () => {
  it("prefixes the provider", () => {
    expect(modelId({ model: "qwen-flash", providerId: "alibaba-qwen" })).toBe("alibaba-qwen/qwen-flash");
  });

  it("leaves a model without a provider alone", () => {
    expect(modelId({ model: "qwen-flash", providerId: "" })).toBe("qwen-flash");
  });
});

describe("formatPrice", () => {
  it("trims trailing zeros", () => {
    expect(formatPrice(0.05)).toBe("0.05");
    expect(formatPrice(10)).toBe("10");
    expect(formatPrice(0.0005)).toBe("0.0005");
  });

  it("marks a missing price", () => {
    expect(formatPrice(null)).toBe("—");
    expect(formatPrice(undefined)).toBe("—");
  });
});
