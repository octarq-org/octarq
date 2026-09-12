import { describe, it, expect } from "vitest";
import { cleanParams } from "./utils";

describe("cleanParams", () => {
  it("removes undefined and null values", () => {
    const input = { a: 1, b: undefined, c: null, d: 2 };
    expect(cleanParams(input)).toEqual({ a: 1, d: 2 });
  });

  it("removes empty and whitespace-only strings", () => {
    const input = { a: "hello", b: "", c: "   ", d: "\t\n" };
    expect(cleanParams(input)).toEqual({ a: "hello" });
  });

  it("trims valid strings", () => {
    const input = { a: "  hello  ", b: "world" };
    expect(cleanParams(input)).toEqual({ a: "hello", b: "world" });
  });

  it("preserves valid numbers, booleans, and objects", () => {
    const obj = { key: "value" };
    const input = { a: 42, b: true, c: obj };
    expect(cleanParams(input)).toEqual({ a: 42, b: true, c: obj });
  });

  it("preserves falsy values like 0 and false", () => {
    const input = { a: 0, b: false };
    expect(cleanParams(input)).toEqual({ a: 0, b: false });
  });

  it("safely handles null or undefined input", () => {
    // @ts-expect-error testing runtime behavior with invalid input
    expect(cleanParams(null)).toEqual({});
    // @ts-expect-error testing runtime behavior with invalid input
    expect(cleanParams(undefined)).toEqual({});
  });

  it("does not mutate the original object", () => {
    const input = { a: 1, b: "  test  ", c: null };
    const result = cleanParams(input);
    expect(result).not.toBe(input);
    expect(input).toEqual({ a: 1, b: "  test  ", c: null });
  });
});
