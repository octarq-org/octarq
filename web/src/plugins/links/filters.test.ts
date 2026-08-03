import { describe, it, expect } from "vitest";
import { parseLinksFilter, buildLinksFilterQuery } from "./filters";

describe("Links url-backed filters pure functions", () => {
  it("1. parses query params correctly and defaults empty query to defaults", () => {
    // Non-empty query
    const parsed = parseLinksFilter("?q=promo&archived=1");
    expect(parsed).toEqual({ q: "promo", archived: true });

    // Empty query
    const defaultParsed = parseLinksFilter("");
    expect(defaultParsed).toEqual({ q: "", archived: false });

    // Query with only search term
    const searchOnly = parseLinksFilter("?q=hello");
    expect(searchOnly).toEqual({ q: "hello", archived: false });

    // Query with archived=0
    const archivedZero = parseLinksFilter("?archived=0");
    expect(archivedZero).toEqual({ q: "", archived: false });
  });

  it("2. serializes filter values to query without including default values", () => {
    // Default values: q is empty, archived is false -> query should be empty
    const queryDefault = buildLinksFilterQuery({ q: "", archived: false });
    expect(queryDefault.toString()).toBe("");

    // Only search term
    const querySearch = buildLinksFilterQuery({ q: "promo", archived: false });
    expect(querySearch.toString()).toBe("q=promo");

    // Only archived
    const queryArchived = buildLinksFilterQuery({ q: "", archived: true });
    expect(queryArchived.toString()).toBe("archived=1");

    // Both
    const queryBoth = buildLinksFilterQuery({ q: "promo", archived: true });
    expect(queryBoth.toString()).toBe("q=promo&archived=1");
  });

  it("3. satisfies roundtrip consistency (parse -> serialize -> parse)", () => {
    const originalFilters = { q: "promo", archived: true };
    const serialized = buildLinksFilterQuery(originalFilters);
    const reParsed = parseLinksFilter(serialized);
    expect(reParsed).toEqual(originalFilters);

    const defaultFilters = { q: "", archived: false };
    const serializedDefault = buildLinksFilterQuery(defaultFilters);
    const reParsedDefault = parseLinksFilter(serializedDefault);
    expect(reParsedDefault).toEqual(defaultFilters);
  });

  it("4. preserves existing query parameters such as create=1", () => {
    const existing = new URLSearchParams("create=1&foo=bar");
    const updated = buildLinksFilterQuery({ q: "promo", archived: true }, existing);
    expect(updated.get("create")).toBe("1");
    expect(updated.get("foo")).toBe("bar");
    expect(updated.get("q")).toBe("promo");
    expect(updated.get("archived")).toBe("1");

    // Deleting filters should keep create=1
    const cleaned = buildLinksFilterQuery({ q: "", archived: false }, updated);
    expect(cleaned.get("create")).toBe("1");
    expect(cleaned.get("foo")).toBe("bar");
    expect(cleaned.get("q")).toBeNull();
    expect(cleaned.get("archived")).toBeNull();
  });
});
