import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const srcDir = path.resolve(__dirname, "../src");
const packagesDir = path.resolve(__dirname, "../../packages");

const colorRegex = /\b(text|bg|border|ring|from|via|to|divide)-(amber|red|rose|green|emerald|lime|yellow|orange|sky|blue|indigo|violet|purple|fuchsia)-[0-9]{2,3}\b/;

// ring-ring, bg-primary. `/* ui-color-ok */` still exempts the semantic status
const brandColorRegex = /\b(text|bg|border|ring|from|via|to|divide|fill|stroke|outline|shadow|accent|caret|decoration)-(sky|blue|indigo|violet|purple)-[0-9]{2,3}\b/;

const seedDeclRegex = /^\s*--(primary|accent-violet)\s*:/;

const brandLiteralRegex = /#(6366f1|4f46e5|4338ca|818cf8|a5b4fc|c7d2fe|eef2ff|8b5cf6|7c5cf6|3b82f6|2563eb)\b|rgba?\(\s*99\s*,\s*102\s*,\s*241/i;

function getFiles(dir) {
  let results = [];
  try {
    const list = fs.readdirSync(dir);
    for (const file of list) {
      const filePath = path.join(dir, file);
      if (file === "generated") continue;
      const stat = fs.statSync(filePath);
      if (stat && stat.isDirectory()) {
        results = results.concat(getFiles(filePath));
      } else if (
        filePath.endsWith(".ts") ||
        filePath.endsWith(".tsx") ||
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
    if (line.includes("ui-not-brand")) return;
    // Checked before the ui-color-ok short-circuit: the brand family is
    if (
      !(isCss && seedDeclRegex.test(line)) &&
      (brandColorRegex.test(line) || brandLiteralRegex.test(line))
    ) {
      console.error(`${relPathOf()}:${index + 1}: hardcoded brand color — use accent tokens (text-accent-fg / bg-accent-soft / border-accent-border / ring-ring / bg-primary) so white-label branding applies: ${line.trim()}`);
      errorCount++;
      return;
    }
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
