import { describe, expect, it } from "vitest";
import * as base from "../../../../packages/plugin-sdk/src/ui/base";
import * as primitives from "../../../../packages/plugin-sdk/src/ui/primitives";
import { CATALOG } from "./catalog";

function isComponent(value: unknown): boolean {
  if (typeof value === "function") return true;
  return typeof value === "object" && value !== null && "$$typeof" in value;
}

function componentNames(mod: Record<string, unknown>): string[] {
  return Object.entries(mod)
    .filter(([name, value]) => isComponent(value) && /^[A-Z]/.test(name))
    .map(([name]) => name);
}

const exported = [...componentNames(base), ...componentNames(primitives)].sort();
const covered = new Set(CATALOG.flatMap((e) => e.covers));

describe("workbench catalog coverage", () => {
  it("resolves the SDK barrels", () => {
    expect(exported.length).toBeGreaterThan(25);
  });

  it("has a catalog entry for every SDK component", () => {
    const missing = exported.filter((name) => !covered.has(name));
    expect(missing, `add a workbench catalog entry covering: ${missing.join(", ")}`).toEqual([]);
  });

  it("claims coverage only for components the SDK actually exports", () => {
    const exportedSet = new Set(exported);
    const phantom = [...covered].filter((name) => !exportedSet.has(name)).sort();
    expect(phantom, `catalog covers names the SDK no longer exports: ${phantom.join(", ")}`).toEqual([]);
  });

  it("has no duplicate entry names and no double-claimed component", () => {
    const names = CATALOG.map((e) => e.name);
    expect(new Set(names).size, "duplicate catalog entry name").toBe(names.length);

    const claims = CATALOG.flatMap((e) => e.covers);
    const doubled = claims.filter((name, i) => claims.indexOf(name) !== i).sort();
    expect(doubled, `component claimed by more than one entry: ${doubled.join(", ")}`).toEqual([]);
  });

  it("keeps every entry inside a declared group", () => {
    const groups = new Set(["layout", "action", "form", "data", "feedback", "overlay"]);
    const bad = CATALOG.filter((e) => !groups.has(e.group)).map((e) => e.name);
    expect(bad).toEqual([]);
  });
});
