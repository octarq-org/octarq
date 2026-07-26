import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const srcDir = path.resolve(__dirname, "../src");

const colorRegex = /\b(text|bg|border|ring|from|via|to|divide)-(amber|red|rose|green|emerald|lime|yellow|orange|sky|blue)-[0-9]{2,3}\b/;

function getFiles(dir) {
  let results = [];
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
  return results;
}

const files = getFiles(srcDir);
let errorCount = 0;

for (const file of files) {
  const content = fs.readFileSync(file, "utf8");
  const lines = content.split("\n");
  lines.forEach((line, index) => {
    if (line.includes("ui-color-ok")) {
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
