import { describe, expect, it } from "vitest";

describe("global network guard", () => {
  it("turns a real fetch into an error naming the method and URL", async () => {
    await expect(fetch("/api/whatever")).rejects.toThrow(/GET \/api\/whatever/);
  });

  it("points at the uncovered mock in the error message", async () => {
    await expect(fetch("/api/whatever", { method: "POST" })).rejects.toThrow(
      /Network request blocked in tests: POST \/api\/whatever.*mock did not cover this call/i,
    );
  });
});
