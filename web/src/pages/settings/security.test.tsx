// @vitest-environment happy-dom
import { describe, it, expect } from "vitest";
import { parseUA } from "./security";

describe("parseUA", () => {
  it("classifies an iPhone as iOS, not macOS", () => {
    const ua =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1";
    expect(parseUA(ua).os).toBe("iOS");
  });

  it("classifies an iPad as iOS, not macOS", () => {
    const ua =
      "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1";
    expect(parseUA(ua).os).toBe("iOS");
  });

  it("classifies Android as Android, not Linux", () => {
    const ua =
      "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36";
    expect(parseUA(ua).os).toBe("Android");
  });

  it("keeps a real desktop macOS as macOS", () => {
    const ua =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";
    expect(parseUA(ua).os).toBe("macOS");
  });

  it("keeps a real desktop Linux as Linux", () => {
    const ua =
      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";
    expect(parseUA(ua).os).toBe("Linux");
  });

  it("classifies Edge as Microsoft Edge, not Chrome", () => {
    const ua =
      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0";
    expect(parseUA(ua).browser).toBe("Microsoft Edge");
  });

  it("classifies Safari as Safari and Chrome as Chrome", () => {
    const safari =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15";
    const chrome =
      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36";
    expect(parseUA(safari).browser).toBe("Safari");
    expect(parseUA(chrome).browser).toBe("Chrome");
  });

  it("marks an empty UA as uaUnknown", () => {
    expect(parseUA("").browserKey).toBe("uaUnknown");
    expect(parseUA("").os).toBe("");
  });

  it("marks an unrecognized UA as uaBrowser", () => {
    const ua = "SomeRandomClient/1.0";
    expect(parseUA(ua).browserKey).toBe("uaBrowser");
    expect(parseUA(ua).browser).toBeUndefined();
  });
});
