import { type z } from "zod";

/**
 * Validates untrusted/external data against a Zod schema with a safe fallback value.
 * Strictly adheres to Octarq network boundary type safety conventions (no bare `as T`).
 */
export function parseWithFallback<T>(schema: z.ZodType<T>, data: unknown, fallback: T): T {
  const result = schema.safeParse(data);
  if (result.success) {
    return result.data;
  }
  return fallback;
}
