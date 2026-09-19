import { describe, it, expect } from "vitest";
import { isNavItemActive } from "./navActive";

describe("isNavItemActive", () => {
  it("activates on the exact path and its child segments", () => {
    expect(isNavItemActive("/mail", "/mail")).toBe(true);
    expect(isNavItemActive("/mail", "/mail/123")).toBe(true);
  });

  it("does not activate a sibling that only shares a string prefix", () => {
    expect(isNavItemActive("/mail", "/maillink")).toBe(false);
    expect(isNavItemActive("/mail", "/mailbox")).toBe(false);
  });

  it("matches query-bearing paths exactly", () => {
    expect(isNavItemActive("/links?create=1", "/links?create=1")).toBe(true);
    expect(isNavItemActive("/links?create=1", "/links")).toBe(false);
  });

  it("treats / as root-only", () => {
    expect(isNavItemActive("/", "/")).toBe(true);
    expect(isNavItemActive("/", "/overview")).toBe(false);
  });
});
