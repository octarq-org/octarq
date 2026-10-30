import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * Guards the section contract every admin page follows: headings come from
 * Panel, and empty-state copy goes in Empty's `reason`/`detail` slots.
 *
 * Both rules exist because the hand-rolled versions fail silently. A section
 * that copies the Panel heading recipe renders correctly but drifts the moment
 * Panel changes. An Empty that takes its message as a child rendered the
 * paragraph inside the 48px icon tile — the text was squeezed, not broken, so
 * nothing failed and five call sites shipped that way.
 */

export type SectionViolation = {
  kind: "hand-rolled-heading" | "empty-block-markup";
  line: number;
  snippet: string;
};

// The Panel h2 recipe. A second copy of it is a section that skipped Panel.
const PANEL_HEADING = "font-display text-[15px]";

const H2_TAG = /<h2\b[^>]*>/g;
// Everything from an opening Empty to its closer. Empty renders detail inside a
// single paragraph and everything else into a 48px tile, so block-level markup
// anywhere inside it either nests paragraphs or crushes the text.
const EMPTY_ELEMENT = /<Empty\b[\s\S]*?<\/Empty>/g;

export function scanSectionSource(raw: string): SectionViolation[] {
  // Blank out block comments, keeping their newlines so line numbers stay
  // true. Comments explain these rules and quote the markup they warn about,
  // and the quotes are not violations.
  const src = raw.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " "));
  const out: SectionViolation[] = [];
  const lineOf = (index: number) => src.slice(0, index).split("\n").length;

  for (const m of src.matchAll(H2_TAG)) {
    if (m[0].includes(PANEL_HEADING)) {
      out.push({ kind: "hand-rolled-heading", line: lineOf(m.index!), snippet: m[0].trim() });
    }
  }

  for (const m of src.matchAll(EMPTY_ELEMENT)) {
    const copy = m[0].match(/<(p|h[1-6])\b/);
    if (copy) {
      out.push({
        kind: "empty-block-markup",
        line: lineOf(m.index! + copy.index!),
        snippet: copy[0],
      });
    }
  }

  return out;
}

const here = path.dirname(fileURLToPath(import.meta.url));
const webSrc = path.resolve(here, "..");

// Routed page modules only. Chrome in shell/ draws its own headers — the
// copilot drawer's title sits in a bordered header row, where a Panel card
// would be the wrong shape.
const SECTION_ROOTS = ["pages", "plugins"].map((dir) => path.join(webSrc, dir));

function sourceFiles(dir: string): string[] {
  let out: string[] = [];
  for (const name of fs.readdirSync(dir)) {
    const full = path.join(dir, name);
    if (fs.statSync(full).isDirectory()) {
      if (name !== "test" && name !== "__tests__") out = out.concat(sourceFiles(full));
    } else if (/\.tsx$/.test(name) && !/\.test\./.test(name)) {
      out.push(full);
    }
  }
  return out;
}

const sectionFiles = SECTION_ROOTS.flatMap(sourceFiles);

describe("scanSectionSource", () => {
  it("flags a section that copies the Panel heading recipe", () => {
    const src = `<div>\n  <h2 className="font-display text-[15px] font-semibold">Plan</h2>\n</div>\n`;
    const hits = scanSectionSource(src);
    expect(hits).toHaveLength(1);
    expect(hits[0].kind).toBe("hand-rolled-heading");
    expect(hits[0].line).toBe(2);
  });

  it("does not flag a page title or a heading that is not the Panel recipe", () => {
    const src = [
      `<h1 className="font-display text-[15px]">Page</h1>`,
      `<h2 className="text-lg font-bold">Subject</h2>`,
      ``,
    ].join("\n");
    expect(scanSectionSource(src)).toHaveLength(0);
  });

  it("flags block-level markup inside an empty state", () => {
    const src = `<Empty>\n  <Key />\n  <p className="text-sm">No keys yet.</p>\n</Empty>\n`;
    const hits = scanSectionSource(src);
    expect(hits).toHaveLength(1);
    expect(hits[0].kind).toBe("empty-block-markup");
    expect(hits[0].line).toBe(3);
  });

  it("does not flag Empty used with the slot API and an icon child", () => {
    const src = [
      `<Empty reason={t("x.title")} detail={t("x.detail")} action={<Button>Add</Button>}>`,
      `  <Key />`,
      `</Empty>`,
      ``,
    ].join("\n");
    expect(scanSectionSource(src)).toHaveLength(0);
  });

  it("does not flag markup quoted in a comment", () => {
    const src = [
      `<Empty reason={t("x.title")} detail={t("x.detail")}>`,
      `  {/* detail renders inside a <p>, so a nested <p> breaks the line. */}`,
      `  <Key />`,
      `</Empty>`,
      ``,
    ].join("\n");
    expect(scanSectionSource(src)).toHaveLength(0);
  });
});

describe("section contract in the page modules", () => {
  it("every section heading comes from Panel", () => {
    const offenders = sectionFiles.flatMap((file) =>
      scanSectionSource(fs.readFileSync(file, "utf8"))
        .filter((v) => v.kind === "hand-rolled-heading")
        .map((v) => `${path.relative(webSrc, file)}:${v.line}`),
    );
    expect(offenders).toEqual([]);
  });

  it("no empty state carries block-level markup", () => {
    const offenders = sectionFiles.flatMap((file) =>
      scanSectionSource(fs.readFileSync(file, "utf8"))
        .filter((v) => v.kind === "empty-block-markup")
        .map((v) => `${path.relative(webSrc, file)}:${v.line}`),
    );
    expect(offenders).toEqual([]);
  });
});
