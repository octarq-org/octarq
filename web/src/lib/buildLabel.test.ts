import { describe, expect, it } from "vitest";
import { buildLabel } from "./buildLabel";

describe("buildLabel", () => {
  it("joins a release tag and a revision", () => {
    expect(buildLabel("v0.9.1", "a5447d8a7cb4")).toBe("v0.9.1 · a5447d8a7cb4");
  });

  it("reports a release tag alone", () => {
    expect(buildLabel("v0.9.1", "")).toBe("v0.9.1");
    expect(buildLabel("v0.9.1", undefined)).toBe("v0.9.1");
  });

  it("reports the revision alone, as a local build carries it", () => {
    expect(buildLabel("", "a5447d8a7cb4")).toBe("a5447d8a7cb4");
    expect(buildLabel(undefined, "a5447d8a7cb4")).toBe("a5447d8a7cb4");
  });

  it("prefers the revision over a pseudo-version that repeats it", () => {
    expect(
      buildLabel("v0.0.0-20261105153446-bb9e6fb26d47+dirty", "bb9e6fb26d47"),
    ).toBe("bb9e6fb26d47");
  });

  it("keeps a pseudo-version when no revision came with it", () => {
    expect(buildLabel("v0.0.0-20261105153446-bb9e6fb26d47", "")).toBe(
      "v0.0.0-20261105153446-bb9e6fb26d47",
    );
  });

  it("reports nothing when the build carries no stamp", () => {
    expect(buildLabel("", "")).toBeNull();
    expect(buildLabel(undefined, undefined)).toBeNull();
  });
});
