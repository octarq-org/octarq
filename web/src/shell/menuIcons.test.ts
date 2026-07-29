import { describe, it, expect } from "vitest";
import { menuIcon } from "./areas";

// An icon string the PLUGIN_ICONS table doesn't know is not an error anywhere:
// the sidebar renders it literally, as text. That is how "book" ended up
// displayed next to Help, and "sparkles" next to Octarq Cloud — the fallback is
// deliberate (it lets a plugin ship an emoji) but it makes a typo'd or
// not-yet-registered key look like intentional copy.
//
// Sidebar placement lives in the Go halves, so that is what this reads. Same
// parse as navI18n.test.ts, and the same reason: assert against the real
// declarations, not a list in the test that would drift from them.
const GO_SOURCES = import.meta.glob(
  "../../../{plugins/**/*.go,internal/**/*.go,examples/**/*.go}",
  { query: "?raw", import: "default", eager: true },
) as Record<string, string>;

const ICON_LITERAL = /Icon:\s*"([^"]+)"/g;

describe("Go-declared menu icons", () => {
  it("are all keys the sidebar can resolve", () => {
    const unknown: string[] = [];
    let seen = 0;
    for (const [path, code] of Object.entries(GO_SOURCES)) {
      if (path.endsWith("_test.go")) continue;
      code.split("\n").forEach((line, i) => {
        for (const m of line.matchAll(ICON_LITERAL)) {
          seen++;
          if (!menuIcon(m[1])) {
            unknown.push(`${path}:${i + 1}  Icon: "${m[1]}"`);
          }
        }
      });
    }
    // A parse that finds nothing would pass vacuously — worse than failing,
    // because it goes quiet exactly when the declaration shape changes.
    expect(seen, "no Icon: literals parsed — did the Go MenuItem shape change?").toBeGreaterThan(3);
    expect(
      unknown,
      `add these to PLUGIN_ICONS in shell/areas.tsx, or use an existing key:\n${unknown.join("\n")}`,
    ).toEqual([]);
  });
});
