import { describe, it, expect } from "vitest";

const GO_SOURCES = import.meta.glob(
  "../../../server/{plugins/**/*.go,internal/**/*.go,app/**/*.go}",
  { query: "?raw", import: "default", eager: true },
) as Record<string, string>;

const SHELL_SOURCES = import.meta.glob("./{App,AreaPanel,AreaRail,AccountMenu,SidebarNav,TopBar,CommandPalette}.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const APP_SOURCE = import.meta.glob("../App.tsx", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const MENU_PATH = /Path:\s*"(\/[^"]*)"/g;
const NAVLINK_TO = /<NavLink\s+[^>]*?to="(\/[^"{]*)"/gs;

function backendPaths(): Set<string> {
  const paths = new Set<string>();
  for (const [file, code] of Object.entries(GO_SOURCES)) {
    if (file.endsWith("_test.go")) continue;
    for (const m of code.matchAll(MENU_PATH)) {
      if (!m[1].startsWith("/api/")) paths.add(m[1]);
    }
  }
  return paths;
}

describe("shell navigation", () => {
  it("does not hardcode a link to a path the Go half announces as a menu", () => {
    const announced = backendPaths();
    expect(announced.size).toBeGreaterThan(0);

    const offenders: string[] = [];
    for (const [file, code] of Object.entries({ ...SHELL_SOURCES, ...APP_SOURCE })) {
      const scannable = code.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (c) =>
        c.replace(/[^\n]/g, " "),
      );
      scannable.split("\n").forEach((line, i) => {
        for (const m of line.matchAll(NAVLINK_TO)) {
          if (announced.has(m[1])) {
            offenders.push(`${file}:${i + 1}  <NavLink to="${m[1]}">`);
          }
        }
      });
    }

    expect(
      offenders,
      `These paths are announced by a Go MenuProvider AND hardcoded in the shell, so ` +
        `they render twice and outlive the plugin that owns them. Let the menu ` +
        `carry them — see the footerItems loop in AreaPanel.tsx.\n${offenders.join("\n")}`,
    ).toEqual([]);
  });
});
