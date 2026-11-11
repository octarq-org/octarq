import React from "react";
import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { roleSatisfies, useCurrentRole, RoleProvider } from "./role";

describe("roleSatisfies", () => {
  it("returns true when no required role is specified", () => {
    expect(roleSatisfies(undefined, "member", false)).toBe(true);
    expect(roleSatisfies("", "member", false)).toBe(true);
  });

  it("returns true when user is an instance admin regardless of role or requirement", () => {
    expect(roleSatisfies("owner", undefined, true)).toBe(true);
    expect(roleSatisfies("owner", "member", true)).toBe(true);
    expect(roleSatisfies("admin", "", true)).toBe(true);
  });

  it("enforces hierarchy: member (1) < admin (2) < owner (3)", () => {
    expect(roleSatisfies("member", "member", false)).toBe(true);
    expect(roleSatisfies("member", "admin", false)).toBe(true);
    expect(roleSatisfies("member", "owner", false)).toBe(true);

    expect(roleSatisfies("admin", "member", false)).toBe(false);
    expect(roleSatisfies("admin", "admin", false)).toBe(true);
    expect(roleSatisfies("admin", "owner", false)).toBe(true);

    expect(roleSatisfies("owner", "member", false)).toBe(false);
    expect(roleSatisfies("owner", "admin", false)).toBe(false);
    expect(roleSatisfies("owner", "owner", false)).toBe(true);
  });

  it("rejects undefined or empty role when a valid rank is required", () => {
    expect(roleSatisfies("member", undefined, false)).toBe(false);
    expect(roleSatisfies("admin", "", false)).toBe(false);
  });

  it("handles unknown required roles gracefully (returns true as unranked)", () => {
    expect(roleSatisfies("custom_role", "member", false)).toBe(true);
  });
});

describe("useCurrentRole", () => {
  it("returns default values when outside RoleProvider", () => {
    const { result } = renderHook(() => useCurrentRole());
    expect(result.current).toEqual({ isInstanceAdmin: false });
  });

  it("reads role and isInstanceAdmin from RoleProvider context", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      React.createElement(RoleProvider, { value: { role: "admin", isInstanceAdmin: false } }, children)
    );
    const { result } = renderHook(() => useCurrentRole(), { wrapper });
    expect(result.current).toEqual({ role: "admin", isInstanceAdmin: false });
  });

  it("reads instance admin status from RoleProvider context", () => {
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      React.createElement(RoleProvider, { value: { isInstanceAdmin: true } }, children)
    );
    const { result } = renderHook(() => useCurrentRole(), { wrapper });
    expect(result.current.isInstanceAdmin).toBe(true);
  });
});
