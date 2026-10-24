import { describe, expect, it } from "vitest";
import { validateArgs, type JsonSchema } from "./validate";

describe("validateArgs", () => {
  it("rejects non-object schemas", () => {
    // @ts-expect-error test invalid schema
    expect(validateArgs(null, {})).toEqual(["$: schema is not an object"]);
  });

  it("rejects non-object args", () => {
    const schema: JsonSchema = { type: "object" };
    expect(validateArgs(schema, null)).toEqual(["$ must be an object"]);
    expect(validateArgs(schema, "str")).toEqual(["$ must be an object"]);
    expect(validateArgs(schema, [1, 2])).toEqual(["$ must be an object"]);
  });

  it("validates required fields", () => {
    const schema: JsonSchema = {
      type: "object",
      required: ["slug", "target"],
      properties: {
        slug: { type: "string" },
        target: { type: "string" },
      },
    };
    expect(validateArgs(schema, { slug: "test" })).toEqual(["$.target is required"]);
    expect(validateArgs(schema, { slug: "test", target: "https://example.com" })).toEqual([]);
  });

  it("validates types (string, boolean, integer, number, array)", () => {
    const schema: JsonSchema = {
      type: "object",
      properties: {
        str: { type: "string" },
        bool: { type: "boolean" },
        int: { type: "integer" },
        num: { type: "number" },
        arr: { type: "array", items: { type: "string" } },
      },
    };

    expect(validateArgs(schema, { str: 123 })).toEqual(["$.str must be a string"]);
    expect(validateArgs(schema, { bool: "not-bool" })).toEqual(["$.bool must be a boolean"]);
    expect(validateArgs(schema, { int: 1.5 })).toEqual(["$.int must be an integer"]);
    expect(validateArgs(schema, { num: "10" })).toEqual(["$.num must be a number"]);
    expect(validateArgs(schema, { arr: [123] })).toEqual(["$.arr[0] must be a string"]);

    expect(
      validateArgs(schema, {
        str: "valid",
        bool: true,
        int: 42,
        num: 3.14,
        arr: ["ok"],
      }),
    ).toEqual([]);
  });

  it("validates string enums", () => {
    const schema: JsonSchema = {
      type: "object",
      properties: {
        choice: { type: "string", enum: ["apple", "banana"] },
      },
    };
    expect(validateArgs(schema, { choice: "orange" })).toEqual([
      '$.choice value "orange" is not allowed',
    ]);
    expect(validateArgs(schema, { choice: "apple" })).toEqual([]);
  });
});
