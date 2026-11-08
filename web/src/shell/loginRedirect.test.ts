// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import {
  safeNextPath,
  resolvePostLoginNavigation,
  performPostLoginNavigation,
} from "./loginRedirect";

describe("safeNextPath", () => {
  it("accepts valid relative dashboard paths", () => {
    expect(safeNextPath("/overview")).toBe("/overview");
    expect(safeNextPath("/settings")).toBe("/settings");
    expect(safeNextPath("/admin/settings")).toBe("/admin/settings");
    expect(safeNextPath("/admin/domains?create=1")).toBe("/admin/domains?create=1");
    expect(safeNextPath("/domains?tab=providers#section")).toBe("/domains?tab=providers#section");
  });

  it("rejects empty or non-string values", () => {
    expect(safeNextPath("")).toBeNull();
    expect(safeNextPath("   ")).toBeNull();
    expect(safeNextPath(null)).toBeNull();
    expect(safeNextPath(undefined)).toBeNull();
  });

  it("rejects open redirect and protocol relative attempts", () => {
    expect(safeNextPath("https://evil.com")).toBeNull();
    expect(safeNextPath("http://evil.com")).toBeNull();
    expect(safeNextPath("//evil.com")).toBeNull();
    expect(safeNextPath("//evil.com/phish")).toBeNull();
    expect(safeNextPath("///evil.com")).toBeNull();
  });

  it("rejects backslash bypasses", () => {
    expect(safeNextPath("/\\evil.com")).toBeNull();
    expect(safeNextPath("\\\\evil.com")).toBeNull();
    expect(safeNextPath("/path\\sub")).toBeNull();
    expect(safeNextPath("/%5cevil.com")).toBeNull();
  });

  it("rejects encoded double-slash attempts", () => {
    expect(safeNextPath("/%2fevil.com")).toBeNull();
  });

  it("rejects javascript: and data: schemes", () => {
    expect(safeNextPath("javascript:alert(1)")).toBeNull();
    expect(safeNextPath("data:text/html,<h1>evil</h1>")).toBeNull();
  });
});

describe("resolvePostLoginNavigation", () => {
  it("resolves relative dashboard paths as SPA routes", () => {
    expect(resolvePostLoginNavigation("/settings")).toEqual({ type: "spa", path: "/settings" });
    expect(resolvePostLoginNavigation("/domains?tab=providers")).toEqual({
      type: "spa",
      path: "/domains?tab=providers",
    });
  });

  it("strips /admin prefix for dashboard SPA paths", () => {
    expect(resolvePostLoginNavigation("/admin/settings")).toEqual({ type: "spa", path: "/settings" });
    expect(resolvePostLoginNavigation("/admin/domains?tab=providers")).toEqual({
      type: "spa",
      path: "/domains?tab=providers",
    });
  });

  it("maps root and login paths to /overview", () => {
    expect(resolvePostLoginNavigation("/")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation("/admin")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation("/admin/")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation("/login")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation("/admin/login")).toEqual({ type: "spa", path: "/overview" });
  });

  it("identifies external routes outside /admin basename", () => {
    expect(resolvePostLoginNavigation("/instance")).toEqual({ type: "external", url: "/instance" });
    expect(resolvePostLoginNavigation("/instance/settings")).toEqual({
      type: "external",
      url: "/instance/settings",
    });
    expect(resolvePostLoginNavigation("/status")).toEqual({ type: "external", url: "/status" });
  });

  it("falls back to /overview when no next param and current is root or login", () => {
    expect(resolvePostLoginNavigation(null, "/")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation(null, "/login")).toEqual({ type: "spa", path: "/overview" });
    expect(resolvePostLoginNavigation(undefined, "/")).toEqual({ type: "spa", path: "/overview" });
  });

  it("falls back to current path when on a specific page without next param", () => {
    expect(resolvePostLoginNavigation(null, "/settings")).toEqual({ type: "spa", path: "/settings" });
    expect(resolvePostLoginNavigation("", "/domains")).toEqual({ type: "spa", path: "/domains" });
  });

  it("falls back safely when next candidate is malicious", () => {
    expect(resolvePostLoginNavigation("https://evil.com", "/")).toEqual({
      type: "spa",
      path: "/overview",
    });
    expect(resolvePostLoginNavigation("//evil.com", "/login")).toEqual({
      type: "spa",
      path: "/overview",
    });
  });
});

describe("performPostLoginNavigation", () => {
  it("navigates via react-router when destination is SPA", () => {
    const navigate = vi.fn();
    performPostLoginNavigation(navigate, "/admin/settings", "/");
    expect(navigate).toHaveBeenCalledWith("/settings", { replace: true });
  });

  it("replaces location when destination is external", () => {
    const navigate = vi.fn();
    const originalLocation = window.location;
    const replaceMock = vi.fn();

    Object.defineProperty(window, "location", {
      configurable: true,
      value: { ...originalLocation, replace: replaceMock },
    });

    try {
      performPostLoginNavigation(navigate, "/instance/console", "/");
      expect(navigate).not.toHaveBeenCalled();
      expect(replaceMock).toHaveBeenCalledWith("/instance/console");
    } finally {
      Object.defineProperty(window, "location", {
        configurable: true,
        value: originalLocation,
      });
    }
  });
});
