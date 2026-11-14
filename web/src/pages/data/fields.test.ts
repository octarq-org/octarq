import { describe, expect, it } from "vitest";
import {
  formatFieldValue,
  formatTimestamp,
  isoToLocalInput,
  parseFieldValue,
  tableFields,
  toFormValue,
} from "./fields";
import type { EntityField } from "./types";

function field(overrides: Partial<EntityField>): EntityField {
  return {
    name: "f",
    type: "string",
    label: "f",
    widget: "text",
    required: false,
    unique: false,
    listColumn: false,
    ...overrides,
  };
}

const labels = { yes: "Yes", no: "No", locale: "en-US" };

describe("parseFieldValue", () => {
  it("sends numbers as numbers, not strings", () => {
    expect(parseFieldValue(field({ type: "integer", widget: "number" }), "3")).toEqual({
      ok: true,
      value: 3,
    });
    expect(parseFieldValue(field({ type: "number", widget: "number" }), "1.5")).toEqual({
      ok: true,
      value: 1.5,
    });
  });

  it("refuses a fraction for an integer field", () => {
    expect(parseFieldValue(field({ type: "integer", widget: "number" }), "2.5")).toEqual({
      ok: false,
      reason: "number",
    });
  });

  it("refuses text in a number field", () => {
    expect(parseFieldValue(field({ type: "number", widget: "number" }), "many")).toEqual({
      ok: false,
      reason: "number",
    });
  });

  it("sends a timestamp as RFC 3339", () => {
    const result = parseFieldValue(
      field({ type: "timestamp", widget: "datetime" }),
      "2026-11-14T10:30",
    );
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(String(result.value)).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
    }
  });

  it("refuses an unparsable date", () => {
    expect(parseFieldValue(field({ type: "timestamp", widget: "datetime" }), "yesterday")).toEqual({
      ok: false,
      reason: "date",
    });
  });

  it("refuses malformed JSON", () => {
    expect(parseFieldValue(field({ type: "json", widget: "json" }), "{nope")).toEqual({
      ok: false,
      reason: "json",
    });
  });

  it("sends JSON as an object, not a string", () => {
    expect(parseFieldValue(field({ type: "json", widget: "json" }), '{"a":1}')).toEqual({
      ok: true,
      value: { a: 1 },
    });
  });

  it("reads a checkbox as a boolean", () => {
    expect(parseFieldValue(field({ type: "boolean", widget: "checkbox" }), true)).toEqual({
      ok: true,
      value: true,
    });
    expect(parseFieldValue(field({ type: "boolean", widget: "checkbox" }), false)).toEqual({
      ok: true,
      value: false,
    });
  });

  it("turns an empty control into null so the caller can decide", () => {
    expect(parseFieldValue(field({}), "")).toEqual({ ok: true, value: null });
  });
});

describe("toFormValue", () => {
  it("renders a stored timestamp in local time for the input", () => {
    const iso = "2026-11-14T02:33:55Z";
    const local = isoToLocalInput(iso);
    // Round-tripping through the input must land on the same instant.
    expect(new Date(local).toISOString()).toBe("2026-11-14T02:33:00.000Z");
  });

  it("pretty-prints stored JSON", () => {
    expect(toFormValue(field({ type: "json", widget: "json" }), '{"a":1}')).toBe(
      '{\n  "a": 1\n}',
    );
  });

  it("leaves a missing value empty", () => {
    expect(toFormValue(field({}), null)).toBe("");
  });
});

describe("formatFieldValue", () => {
  it("renders booleans with the supplied labels", () => {
    expect(formatFieldValue(field({ type: "boolean" }), true, labels)).toBe("Yes");
    expect(formatFieldValue(field({ type: "boolean" }), false, labels)).toBe("No");
  });

  it("renders an empty cell for null", () => {
    expect(formatFieldValue(field({}), null, labels)).toBe("");
  });

  it("falls back to the raw value for an unparsable timestamp", () => {
    expect(formatTimestamp("not-a-date", "en-US")).toBe("not-a-date");
  });
});

describe("tableFields", () => {
  it("shows the declared list columns in declaration order", () => {
    const a = field({ name: "a", listColumn: true });
    const b = field({ name: "b" });
    const c = field({ name: "c", listColumn: true });
    expect(tableFields([a, b, c]).map((f) => f.name)).toEqual(["a", "c"]);
  });

  it("shows every field when none is marked", () => {
    const a = field({ name: "a" });
    const b = field({ name: "b" });
    expect(tableFields([a, b]).map((f) => f.name)).toEqual(["a", "b"]);
  });
});
