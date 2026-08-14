import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const srcDir = path.resolve(__dirname, "../src");
const packagesDir = path.resolve(__dirname, "../../packages");

const colorRegex = /\b(text|bg|border|ring|from|via|to|divide)-(amber|red|rose|green|emerald|lime|yellow|orange|sky|blue|indigo|violet|purple|fuchsia)-[0-9]{2,3}\b/;

function getFiles(dir) {
  let results = [];
  try {
    const list = fs.readdirSync(dir);
    for (const file of list) {
      const filePath = path.join(dir, file);
      const stat = fs.statSync(filePath);
      if (stat && stat.isDirectory()) {
        results = results.concat(getFiles(filePath));
      } else if (filePath.endsWith(".ts") || filePath.endsWith(".tsx")) {
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
  const content = fs.readFileSync(file, "utf8");
  const lines = content.split("\n");
  lines.forEach((line, index) => {
    if (line.includes("ui-color-ok")) {
      if (markerSitsInChildren(line)) {
        const relPath = path.relative(path.resolve(__dirname, ".."), file);
        console.error(`${relPath}:${index + 1}: /* ui-color-ok */ sits after a closed tag and would render as page text — move it inside the opening tag`);
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
