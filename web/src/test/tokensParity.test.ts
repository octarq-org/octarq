import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SEED_TOKENS, TOKENS, themeAlias } from "@octarq/plugin-sdk";

/**
 * Locks the design-token inventory (packages/plugin-sdk/src/tokens) to the
 * stylesheet that declares it (web/src/styles.css).
 *
 * The inventory exists so a consumer — the UI workbench's token inspector, or a
 * plugin picking a token by name — can enumerate tokens instead of scraping CSS.
 * That only holds if the two cannot drift, so every assertion below is a SET
 * EQUALITY: the stylesheet may neither declare a token the inventory does not
 * know about, nor omit one it claims. A one-way check would let the stylesheet
 * grow a token the inventory never learns about.
 *
 * Values are not compared and are not stored in the inventory on purpose — the
 * stylesheet is their only source (see the module header). Presence, not value,
 * is what has to stay in sync.
 */
const here = path.dirname(fileURLToPath(import.meta.url));
const css = fs
  .readFileSync(path.resolve(here, "../styles.css"), "utf8")
  .replace(/\/\*[\s\S]*?\*\//g, "");

function declaredNames(re: RegExp): string[] {
  const block = css.match(re);
  if (!block) throw new Error(`token block not found in styles.css: ${re}`);
  const names: string[] = [];
  for (const d of block[1].matchAll(/(?<name>--[A-Za-z0-9-]+)\s*:/g)) {
    names.push(d.groups!.name);
  }
  return names.sort();
}

const rootNames = declaredNames(/:root\s*\{([\s\S]*?)\n\}/);
const darkNames = declaredNames(/\.dark\s*\{([\s\S]*?)\n\}/);
const themeNames = declaredNames(/@theme inline\s*\{([\s\S]*?)\n\}/);

const expected = (predicate: (t: (typeof TOKENS)[number]) => boolean) =>
  TOKENS.filter(predicate)
    .map((t) => t.name)
    .sort();

describe("design-token inventory vs styles.css", () => {
  it("matches the :root block", () => {
    expect(rootNames).toEqual(expected((t) => t.root));
  });

  it("matches the .dark block", () => {
    expect(darkNames).toEqual(expected((t) => t.dark));
  });

  it("matches the @theme inline block", () => {
    const aliases = TOKENS.map(themeAlias).filter((n): n is string => n !== null).sort();
    expect(themeNames).toEqual(aliases);
  });

  it("keeps the rebrand write surface to exactly the three seed literals", () => {
    // brand.tsx applyAccents overrides only --primary and --accent-violet; every
    // other brand-tinted token derives from --primary. A fourth seed would mean a
    // rebrand silently leaves something behind.
    expect(SEED_TOKENS.map((t) => t.name)).toEqual([
      "--primary",
      "--accent-violet",
      "--primary-foreground",
    ]);
  });
});
