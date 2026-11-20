import { describe, it, expect } from "vitest";
import { authErrorKey, authErrorCodes, isVerifiedFlag } from "./authErrors";
import { en } from "../i18n/en";

function lookup(dict: unknown, key: string): unknown {
  return key.split(".").reduce<unknown>(
    (node, part) =>
      node && typeof node === "object" ? (node as Record<string, unknown>)[part] : undefined,
    dict,
  );
}

describe("authErrorKey", () => {
  it("maps every declared code to a string that exists in the dictionary", () => {
    for (const code of authErrorCodes()) {
      const key = authErrorKey(code);
      expect(key, `${code} has no key`).toBeTruthy();
      expect(typeof lookup(en, key!), `${key} missing from en`).toBe("string");
    }
  });

  it("falls back to the generic message rather than echoing an unknown code", () => {
    expect(authErrorKey("<script>alert(1)</script>")).toBe(authErrorKey("login_failed"));
    expect(authErrorKey("totally_made_up")).toBe(authErrorKey("login_failed"));
  });

  it("stays quiet when there is no error at all", () => {
    expect(authErrorKey(null)).toBeNull();
    expect(authErrorKey(undefined)).toBeNull();
    expect(authErrorKey("")).toBeNull();
  });
});

describe("isVerifiedFlag", () => {
  it("accepts the spelling the server actually sends", () => {
    expect(isVerifiedFlag("1")).toBe(true);
  });

  it("still accepts the spelling the page used to check for", () => {
    expect(isVerifiedFlag("true")).toBe(true);
  });

  it("rejects everything else", () => {
    expect(isVerifiedFlag("0")).toBe(false);
    expect(isVerifiedFlag("yes")).toBe(false);
    expect(isVerifiedFlag(null)).toBe(false);
    expect(isVerifiedFlag(undefined)).toBe(false);
  });
});
