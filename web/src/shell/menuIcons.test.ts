import { describe, it, expect } from "vitest";
import { menuIcon } from "./areas";

const GO_SOURCES = import.meta.glob(
  "../../../server/{plugins/**/*.go,internal/**/*.go,examples/**/*.go}",
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
    expect(seen, "no Icon: literals parsed — did the Go MenuItem shape change?").toBeGreaterThan(3);
    expect(
      unknown,
      `add these to PLUGIN_ICONS in shell/areas.tsx, or use an existing key:\n${unknown.join("\n")}`,
    ).toEqual([]);
  });
});

describe("the ontology icon allowlist", () => {
  it("holds only keys the sidebar can resolve", () => {
    const source = GO_SOURCES["../../../server/plugins/ontology/icons.go"];
    expect(source, "icons.go was not parsed — did the file move?").toBeTruthy();
    const keys = [...source.matchAll(/"([a-z0-9-]+)":\s*true/g)].map((m) => m[1]);
    expect(keys.length, "no allowlist keys parsed — did the map shape change?").toBeGreaterThan(3);
    const unknown = keys.filter((k) => !menuIcon(k));
    expect(
      unknown,
      `remove these from server/plugins/ontology/icons.go, or add them to PLUGIN_ICONS:\n${unknown.join("\n")}`,
    ).toEqual([]);
  });
});
