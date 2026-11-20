import { describe, it, expect } from "vitest";
import fs from "node:fs";
import path from "node:path";

const css = fs.readFileSync(path.resolve(__dirname, "styles.css"), "utf8");

function block(selector: string): string {
  const start = css.indexOf(selector + " {");
  expect(start, `${selector} block not found in styles.css`).toBeGreaterThan(-1);
  const open = css.indexOf("{", start);
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === "{") depth++;
    else if (css[i] === "}" && --depth === 0) return css.slice(open + 1, i);
  }
  throw new Error(`unterminated ${selector} block`);
}

function declared(body: string, name: string): string {
  const m = body.match(new RegExp(`^\\s*${name}\\s*:([^;]+);`, "m"));
  return m ? m[1].trim() : "";
}

const DERIVED = [
  "--primary-hover",
  "--primary-solid",
  "--accent-indigo",
  "--accent-fg",
  "--accent-soft",
  "--accent-border",
  "--ring",
  "--gradient-primary",
  "--info-bg",
  "--info-border",
  "--info-fg",
];

describe("brand accent tokens derive from the --primary seed", () => {
  for (const selector of [":root", ".dark"]) {
    const body = block(selector);
    for (const token of DERIVED) {
      const value = declared(body, token);
      if (selector === ".dark" && value === "") continue;
      it(`${selector} ${token} references var(--primary)`, () => {
        expect(value).not.toBe("");
        expect(
          value,
          `${token} is a literal in ${selector}; it will not follow a white-label brand color`,
        ).toMatch(/var\(--primary(?:-solid)?\)/);
      });
    }
  }

  it("only the documented seeds are literal colors", () => {
    const seeds = ["--primary", "--accent-violet", "--primary-foreground"];
    for (const selector of [":root", ".dark"]) {
      const body = block(selector);
      for (const line of body.split("\n")) {
        const m = line.match(/^\s*(--[a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;/);
        if (!m) continue;
        const [, name, hex] = m;
        if (seeds.includes(name)) continue;
        expect(
          /^--(accent|primary|ring|info|gradient)/.test(name),
          `${selector} ${name}: ${hex} is a brand-tinted literal — derive it from var(--primary)`,
        ).toBe(false);
      }
    }
  });
});

describe("applyAccents writes only the seeds", () => {
  const source = fs.readFileSync(path.resolve(__dirname, "brand.tsx"), "utf8");
  const writes = source.split("\n").filter((l) => l.includes("setProperty("));

  it("does not set derived tokens from JS", () => {
    for (const token of DERIVED) {
      const offender = writes.find((l) => l.includes(`"${token}"`) || l.includes(`'${token}'`));
      expect(offender, `applyAccents still sets ${token}; let styles.css derive it`).toBeUndefined();
    }
  });

  it("sets the two operator seeds", () => {
    expect(writes.join("\n")).toMatch(/setProperty\(['"]--primary['"]/);
    expect(writes.join("\n")).toMatch(/setProperty\(['"]--accent-violet['"]/);
  });
});
