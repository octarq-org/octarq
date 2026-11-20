import { describe, it, expect } from "vitest";

const sources = import.meta.glob("./**/*.{ts,tsx}", { query: "?raw", import: "default", eager: true }) as Record<
  string,
  string
>;

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
