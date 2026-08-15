// Guard for the white-label re-theming contract.
//
// Branding works by overriding ONE seed token (--primary, plus --accent-violet
// for the gradient end) inline on <html>. Every other brand-tinted token must be
// mixed from that seed in styles.css, or it keeps the octarq indigo on a
// rebranded instance — which is exactly what happened: --primary-hover,
// --accent-fg and --info-* were literal hexes, so hover states, accent text and
// info badges across the settings pages stayed blue while buttons re-themed.
//
// These assertions are static reads of styles.css on purpose. jsdom does not
// evaluate color-mix(), so a getComputedStyle-based test here would pass on a
// stylesheet that reintroduced literals. Reading the declarations catches it.
import { describe, it, expect } from "vitest";
import fs from "node:fs";
import path from "node:path";

const css = fs.readFileSync(path.resolve(__dirname, "styles.css"), "utf8");

// block extracts the declarations of a top-level rule (`:root` / `.dark`) so an
// assertion about the light palette can't be satisfied by the dark one.
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

// declared returns the value of `--name` in a block, or "" when the block does
// not set it (inheriting the :root formula is a valid way to derive).
function declared(body: string, name: string): string {
  const m = body.match(new RegExp(`^\\s*${name}\\s*:([^;]+);`, "m"));
  return m ? m[1].trim() : "";
}

// Tokens that MUST resolve from --primary. Adding a brand-tinted token to
// styles.css means adding it here too.
const DERIVED = [
  "--primary-hover",
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
      // An absent declaration inherits the :root formula — already derived.
      if (selector === ".dark" && value === "") continue;
      it(`${selector} ${token} references var(--primary)`, () => {
        expect(value).not.toBe("");
        expect(
          value,
          `${token} is a literal in ${selector}; it will not follow a white-label brand color`,
        ).toContain("var(--primary)");
      });
    }
  }

  it("only the documented seeds are literal colors", () => {
    // --accent-violet is the second seat the operator sets (gradient end), and
    // --primary-foreground is the text on the fill, not a tint of it.
    const seeds = ["--primary", "--accent-violet", "--primary-foreground"];
    for (const selector of [":root", ".dark"]) {
      const body = block(selector);
      for (const line of body.split("\n")) {
        const m = line.match(/^\s*(--[a-z0-9-]+)\s*:\s*(#[0-9a-fA-F]{3,8})\s*;/);
        if (!m) continue;
        const [, name, hex] = m;
        if (seeds.includes(name)) continue;
        // Non-brand palette entries (surfaces, borders, semantic status) are
        // allowed literals — only the brand-tinted families are constrained.
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
  // Only the setProperty call sites matter. Slicing a region of the file instead
  // would let the explanatory comment naming these tokens fail the assertion.
  const writes = source.split("\n").filter((l) => l.includes("setProperty("));

  it("does not set derived tokens from JS", () => {
    // A token set in both CSS and JS drifts, and the JS copy silently wins.
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
