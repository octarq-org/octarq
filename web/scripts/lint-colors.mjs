import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const srcDir = path.resolve(__dirname, "../src");
const packagesDir = path.resolve(__dirname, "../../packages");

const colorRegex = /\b(text|bg|border|ring|from|via|to|divide)-(amber|red|rose|green|emerald|lime|yellow|orange|sky|blue|indigo|violet|purple|fuchsia)-[0-9]{2,3}\b/;

// The BRAND family is not exemptible. A literal indigo/violet/blue is the brand
// accent spelled out, and the white-label plugin re-themes by overriding the
// --primary seed — a hardcoded hex can't follow it, which is exactly how half
// the settings pages stayed indigo on a rebranded instance. Use the accent
// tokens instead: text-accent-fg, bg-accent-soft, border-accent-border,
// ring-ring, bg-primary. `/* ui-color-ok */` still exempts the semantic status
// families (red/amber/green/…), where a literal hue IS the meaning.
const brandColorRegex = /\b(text|bg|border|ring|from|via|to|divide|fill|stroke|outline|shadow|accent|caret|decoration)-(sky|blue|indigo|violet|purple)-[0-9]{2,3}\b/;

// Brand hexes have the same problem in a raw style/arbitrary value, where no
// Tailwind class name gives them away (e.g. shadow-[0_0_0_3px_rgba(99,102,241,.18)]).
// The seed declarations themselves are the one place a brand hex belongs: they
// DEFINE the brand colour rather than consuming it, and the white-label plugin
// overrides them at runtime. Everything else must derive from them.
const seedDeclRegex = /^\s*--(primary|accent-violet)\s*:/;

const brandLiteralRegex = /#(6366f1|4f46e5|4338ca|818cf8|a5b4fc|c7d2fe|eef2ff|8b5cf6|7c5cf6|3b82f6|2563eb)\b|rgba?\(\s*99\s*,\s*102\s*,\s*241/i;

function getFiles(dir) {
  let results = [];
  try {
    const list = fs.readdirSync(dir);
    for (const file of list) {
      const filePath = path.join(dir, file);
      // Generated clients are not hand-written UI: their colours come from
      // upstream doc strings, and editing them here would be undone by the next
      // regeneration. octarq has no such directory today; the rule is here so the
      // two repos' lints stay identical.
      if (file === "generated") continue;
      const stat = fs.statSync(filePath);
      if (stat && stat.isDirectory()) {
        results = results.concat(getFiles(filePath));
      } else if (
        filePath.endsWith(".ts") ||
        filePath.endsWith(".tsx") ||
        // .css too: the brand rule has to cover component classes like `.input`
        // and `::selection`, whose colours live in the stylesheet and never
        // appear as a Tailwind class. A hardcoded focus ring there is invisible
        // to a .tsx-only scan — that is how `.input:focus` kept an indigo glow
        // on every settings form after the class-level call sites were fixed.
        filePath.endsWith(".css")
      ) {
        results.push(filePath);
      }
    }
  } catch (e) {}
  return results;
}

let files = getFiles(srcDir);
try {
  const pkgs = fs.readdirSync(packagesDir);
  for (const pkg of pkgs) {
    const pkgSrc = path.join(packagesDir, pkg, "src");
    if (fs.existsSync(pkgSrc)) {
      files = files.concat(getFiles(pkgSrc));
    }
  }
} catch (e) {}
let errorCount = 0;

// A `/* ui-color-ok */` marker only exempts the line when it sits in a JS
// comment position: inside an opening tag (between attributes) or in a JS
// expression. When the marker appears AFTER a tag has already closed on the
// same line — whether self-closing (`/>`), a closing tag (`</tag>`), or a
// plain opening-tag end (`>`) — it is a JSX children text node and renders as
// literal page text. That must be an error, not an exemption.
//
// Detection: strip string literals so attribute values can't fake a tag end,
// then iteratively strip balanced `{...}` groups so `>` inside JS expressions
// (`a > b`, `() => x`, template interpolations) can't fake one either. A tag
// has closed when the last `>` on the line sits after the last `<`.
function markerSitsInChildren(line) {
  const idx = line.indexOf("ui-color-ok");
  if (idx === -1) return false;
  let before = line
    .slice(0, idx)
    .replace(/"[^"]*"/g, '""')
    .replace(/'[^']*'/g, "''")
    .replace(/`[^`]*`/g, "``");
  let prev;
  do {
    prev = before;
    before = before.replace(/\{[^{}]*\}/g, "");
  } while (before !== prev);
  const lastLt = before.lastIndexOf("<");
  const lastGt = before.lastIndexOf(">");
  return lastGt > lastLt;
}

for (const file of files) {
  const isCss = file.endsWith(".css");
  const content = fs.readFileSync(file, "utf8");
  const lines = content.split("\n");
  lines.forEach((line, index) => {
    const relPathOf = () => path.relative(path.resolve(__dirname, ".."), file);
    // `/* ui-not-brand */` marks a literal that carries NO UI semantics at all —
    // neither brand accent nor status — so it exempts BOTH rules. The cases it
    // covers span both: the branding editor naming octarq's default seed to prime
    // its own colour picker, and fixed external palettes such as the xterm ANSI 16,
    // where `red`/`green`/`yellow` mean ANSI red/green/yellow and not
    // danger/success/warning. It is not a licence to colour chrome by hand.
    //
    // Keep this identical to octarq-pro's portal/scripts/lint-colors.mjs. The two
    // drifted once — the marker exempted one rule here and both there — which
    // makes a marker that reads as legitimate in one repo silently insufficient
    // in the other.
    if (line.includes("ui-not-brand")) return;
    // Checked before the ui-color-ok short-circuit: the brand family is
    // unexemptible, so that marker must not buy it a pass.
    if (
      !(isCss && seedDeclRegex.test(line)) &&
      (brandColorRegex.test(line) || brandLiteralRegex.test(line))
    ) {
      console.error(`${relPathOf()}:${index + 1}: hardcoded brand color — use accent tokens (text-accent-fg / bg-accent-soft / border-accent-border / ring-ring / bg-primary) so white-label branding applies: ${line.trim()}`);
      errorCount++;
      return;
    }
    // The status rule matches Tailwind class names, which a stylesheet has none
    // of; only the brand rule above applies to CSS.
    if (isCss) return;
    if (line.includes("ui-color-ok")) {
      if (markerSitsInChildren(line)) {
        console.error(`${relPathOf()}:${index + 1}: /* ui-color-ok */ sits after a closed tag and would render as page text — move it inside the opening tag`);
        errorCount++;
      }
      return;
    }
    if (colorRegex.test(line)) {
      const relPath = path.relative(path.resolve(__dirname, ".."), file);
      console.error(`${relPath}:${index + 1}: ${line.trim()}`);
      errorCount++;
    }
  });
}

if (errorCount > 0) {
  console.error(`\nFound ${errorCount} raw status color usage(s). Use semantic status tokens/components or add /* ui-color-ok */ if legitimate.`);
  process.exit(1);
} else {
  console.log("No raw status color violations found.");
  process.exit(0);
}
