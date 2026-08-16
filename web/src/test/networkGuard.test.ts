// Guards the guard: proves the global fetch stub installed by setup.ts (the
// file next to this one) really turns real network calls into errors.
//
// Do NOT "fix" this file by re-implementing fetch interception locally — that
// would test nothing. It must go red whenever the stub in setup.ts is removed
// or short-circuited, and green exactly when the stub blocks.
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
