/**
 * Minimal JSON-Schema validator for tool arguments.
 * Total function: any value in, string list out. Never throws.
 */

export type JsonSchema = {
  type?: string;
  properties?: Record<string, JsonSchema>;
  required?: string[];
  items?: JsonSchema;
  additionalProperties?: JsonSchema | boolean;
  enum?: unknown[];
  description?: string;
  format?: string;
  $schema?: string;
};

/** Validate a parsed args value against a tool schema. Empty list = valid. */
export function validateArgs(
  schema: JsonSchema,
  value: unknown,
  path = "$",
): string[] {
  if (typeof schema !== "object" || schema === null) {
    return [`${path}: schema is not an object`];
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return [`${path} must be an object`];
  }
  return checkObject(schema, value as Record<string, unknown>, path);
}

function checkObject(
  schema: JsonSchema,
  obj: Record<string, unknown>,
  path: string,
): string[] {
  const errors: string[] = [];
  const props = schema.properties ?? {};
  for (const name of schema.required ?? []) {
    if (!(name in obj)) {
      errors.push(`${path}.${name} is required`);
    }
  }
  for (const [name, val] of Object.entries(obj)) {
    const prop = props[name];
    if (prop === undefined) continue; // unknown field: forward compatibility
    if (typeof prop !== "object" || prop === null) continue;
    errors.push(...checkValue(prop, val, `${path}.${name}`));
  }
  return errors;
}

function checkValue(
  prop: JsonSchema,
  val: unknown,
  path: string,
): string[] {
  switch (prop.type) {
    case "string": {
      if (typeof val !== "string") return [`${path} must be a string`];
      if (Array.isArray(prop.enum) && !prop.enum.includes(val)) {
        return [`${path} value ${JSON.stringify(val)} is not allowed`];
      }
      return [];
    }
    case "boolean":
      return typeof val === "boolean" ? [] : [`${path} must be a boolean`];
    case "integer":
      return typeof val === "number" && Number.isInteger(val)
        ? []
        : [`${path} must be an integer`];
    case "number":
      return typeof val === "number" ? [] : [`${path} must be a number`];
    case "array": {
      if (!Array.isArray(val)) return [`${path} must be an array`];
      const errors: string[] = [];
      const items = prop.items ?? {};
      for (let i = 0; i < val.length; i++) {
        errors.push(...checkValue(items, val[i], `${path}[${i}]`));
      }
      return errors;
    }
    case "object": {
      if (typeof val !== "object" || val === null || Array.isArray(val)) {
        return [`${path} must be an object`];
      }
      const obj = val as Record<string, unknown>;
      if (prop.properties !== undefined) {
        return checkObject(prop, obj, path);
      }
      return [];
    }
    default:
      return [];
  }
}
