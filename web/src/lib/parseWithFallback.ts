import { type z } from "zod";

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
