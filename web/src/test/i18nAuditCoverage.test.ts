import { describe, expect, it } from "vitest";
import { scanTsxSource } from "../../scripts/i18n-audit-core.mjs";

describe("scanTsxSource", () => {
  it("catches a hardcoded string in a copy attribute (label)", () => {
    const src = `<Field label="Untranslated" />\n`;
    const hits = scanTsxSource(src, "sample.tsx");
    expect(hits).toHaveLength(1);
    expect(hits[0].kind).toBe("label");
    expect(hits[0].text).toBe("Untranslated");
    expect(hits[0].line).toBe(1);
  });

  it("catches a hardcoded string in a JSX expression container", () => {
    const src = `<div>{"Untranslated"}</div>\n`;
    const hits = scanTsxSource(src, "sample.tsx");
    expect(hits).toHaveLength(1);
    expect(hits[0].kind).toBe("JSXExpression");
    expect(hits[0].text).toBe("Untranslated");
  });

  it("does not flag an already-translated label expression", () => {
    const src = `<Field label={t("x.y")} />\n`;
    expect(scanTsxSource(src, "sample.tsx")).toHaveLength(0);
  });

  it("does not flag identifier attributes", () => {
    const src = `<div id="main" />\n`;
    expect(scanTsxSource(src, "sample.tsx")).toHaveLength(0);
  });

  it("reports the correct line for each hit", () => {
    const src = `<div className="page">\n  <Field label="First" />\n  <Field label="Second" />\n</div>\n`;
    const hits = scanTsxSource(src, "sample.tsx");
    expect(hits.map((h) => `${h.line}:${h.kind}:${h.text}`)).toEqual([
      "2:label:First",
      "3:label:Second",
    ]);
  });
});