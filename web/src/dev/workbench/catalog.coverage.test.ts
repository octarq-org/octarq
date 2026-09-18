import { describe, expect, it } from "vitest";
import * as base from "../../../../packages/plugin-sdk/src/ui/base";
import * as primitives from "../../../../packages/plugin-sdk/src/ui/primitives";
import { CATALOG } from "./catalog";

/**
 * The workbench's coverage contract.
 *
 * The catalog only earns its name if it previews everything: a component added to
 * the SDK with no catalog entry is invisible to the one surface meant to show the
 * whole kit, and nobody notices because nothing fails. So this reads the SDK's own
 * barrels at runtime and requires an entry for every exported component.
 *
 * "Component" is detected structurally, not from a hand-maintained list: an export
 * that is a function and starts with an uppercase letter. That excludes the
 * lowercase helpers (`cn`, `timeAgo`, `useTableDensity`, `formErrorMessage`), the
 * variant builders (`buttonVariants`), and non-functions (`TIER_LABEL`,
 * `formErrorStatusKeys`). Parts with no standalone meaning are folded into a
 * parent entry through its `covers` list.
 */
// A React component is either a plain function or a forwardRef/memo result — and
// those are OBJECTS carrying a $$typeof symbol, not functions. Checking only for
// `typeof === "function"` silently skipped Button, Badge and Alert (all
// forwardRef), which is how this test first passed while claiming to cover them.
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
    // Guard against the whole suite passing vacuously if an import path rots and
    // both sides come back empty.
    expect(exported.length).toBeGreaterThan(25);
  });

  it("has a catalog entry for every SDK component", () => {
    const missing = exported.filter((name) => !covered.has(name));
    expect(missing, `add a workbench catalog entry covering: ${missing.join(", ")}`).toEqual([]);
  });

  it("claims coverage only for components the SDK actually exports", () => {
    // Catches a `covers` entry that went stale after a rename or removal, which
    // would otherwise silently stop covering the real component.
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
