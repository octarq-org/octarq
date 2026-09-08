import { type z } from "zod";

/**
 * Validates untrusted/external data against a Zod schema with a safe fallback value.
 * Strictly adheres to Octarq network boundary type safety conventions (no bare `as T`).
 * If validation fails, calls optional onError callback or logs a warning, and returns the fallback value.
 */
export function parseWithFallback<T>(
  schema: z.ZodType<T>,
  data: unknown,
  fallback: T,
  onError?: (err: z.ZodError) => void,
): T {
  const result = schema.safeParse(data);
  if (result.success) {
    return result.data;
  }
  if (onError) {
    onError(result.error);
  } else {
    console.warn("Zod schema validation fallback triggered:", result.error.format());
  }
  return fallback;
}
