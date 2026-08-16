import { describe, it, expect } from "vitest";

// Sidebar placement comes from the Go halves and nothing else
// (website/src/content/docs/writing-a-plugin.md): a plugin's Menus() is the
// only source of id, path, label, category and icon.
// The shell had drifted from that in the footer, which hardcoded
//
//   <NavLink to="/help">
//
// while the help plugin ALSO announced {ID: "help", Path: "/help",
// Category: "footer"}. Both rendered, so Help appeared twice — once as a rail
// link, once inside the resources menu. The duplicate is the visible symptom;
// the real defect is that a hardcoded link survives its plugin, pointing at a
// route nothing serves in a build that dropped the feature.
//
// This reads the real declarations on both sides rather than a list here, for
// the same reason menuIcons.test.ts does: a list would drift from what it guards.
const GO_SOURCES = import.meta.glob(
  "../../../{plugins/**/*.go,internal/**/*.go,app/**/*.go}",
  { query: "?raw", import: "default", eager: true },
) as Record<string, string>;

const SHELL_SOURCES = import.meta.glob("./{App,AreaPanel,TopBar,CommandPalette}.tsx", {
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
      // Only MenuItem paths, not the many huma.Register API routes.
      if (!m[1].startsWith("/api/")) paths.add(m[1]);
    }
  }
  return paths;
}

describe("shell navigation", () => {
  it("does not hardcode a link to a path the Go half announces as a menu", () => {
    const announced = backendPaths();
    expect(announced.size).toBeGreaterThan(0); // the glob resolved something

    const offenders: string[] = [];
    for (const [file, code] of Object.entries({ ...SHELL_SOURCES, ...APP_SOURCE })) {
      // Blank out comments while keeping line numbering intact — a comment
      // explaining why a link ISN'T hardcoded any more must not read as one.
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
