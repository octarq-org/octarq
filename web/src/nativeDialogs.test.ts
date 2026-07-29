import { describe, it, expect } from "vitest";

// The native dialogs are gone from this app: alert() was replaced by the toast
// system, confirm() by confirmDialog(), and prompt() never had a home. They
// render unstyled OS chrome and block the entire event loop, so one careless
// reintroduction undoes the whole surface.
//
// This guards the ban rather than any single conversion, because the failure is
// invisible in review: a native confirm() looks exactly like the SDK one at the
// call site, minus an await — and the missing await silently makes the guard
// pass, since a Promise is truthy.
//
// Sources come from import.meta.glob rather than node:fs — the app's tsconfig
// carries no node types, and Vite already knows the module graph.
const sources = import.meta.glob("./**/*.{ts,tsx}", { query: "?raw", import: "default", eager: true }) as Record<
  string,
  string
>;

// `confirmDialog(` must not match, so the call may not be preceded by an
// identifier character or a dot (which would make it a method or a longer name).
const NATIVE_CALL = /(?<![\w.])(?:window\.)?(?:alert|confirm|prompt)\s*\(/;

describe("native browser dialogs", () => {
  it("are not used anywhere in the app", () => {
    const offenders: string[] = [];
    for (const [path, code] of Object.entries(sources)) {
      if (/\.test\.tsx?$/.test(path)) continue;
      code.split("\n").forEach((line, i) => {
        if (NATIVE_CALL.test(line)) offenders.push(`${path}:${i + 1}  ${line.trim()}`);
      });
    }
    expect(offenders, `use toast / confirmDialog instead:\n${offenders.join("\n")}`).toEqual([]);
  });
});
