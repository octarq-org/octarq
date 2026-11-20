import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SEED_TOKENS, TOKENS, themeAlias } from "@octarq/plugin-sdk";

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
    expect(SEED_TOKENS.map((t) => t.name)).toEqual([
      "--primary",
      "--accent-violet",
      "--primary-foreground",
    ]);
  });
});
