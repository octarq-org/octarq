import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webSrc = path.resolve(__dirname, "../src");
const sdkSrc = path.resolve(__dirname, "../../packages/plugin-sdk/src");
const helloPluginDir = path.resolve(__dirname, "../../server/examples/plugin-hello/web");

// Modal/drawer/palette backdrops are genuine elevated overlays; every other UI surface
// must remain flat on the 3-surface luminance stack without backdrop-blur.
const OVERLAY_BLUR_ALLOWLIST = new Set([
  "packages/plugin-sdk/src/ui/base/dialog.tsx",
  "web/src/shell/CommandPalette.tsx",
  "web/src/pages/notifications/InboxDrawer.tsx",
  "web/src/copilot/CopilotDrawer.tsx",
  "web/src/shell/AICopilotDrawer.tsx",
]);

function walk(dir) {
  let out = [];
  let entries;
  try {
    entries = fs.readdirSync(dir);
  } catch {
    return out;
  }
  for (const name of entries) {
    const full = path.join(dir, name);
    if (fs.statSync(full).isDirectory()) {
      if (name === "__tests__" || name === "test" || name === "node_modules" || name === "dist") continue;
      out = out.concat(walk(full));
    } else if (/\.(ts|tsx|css)$/.test(name) && !/\.test\./.test(name)) {
      out.push(full);
    }
  }
  return out;
}

const repoRoot = path.resolve(__dirname, "../..");
const files = [...walk(webSrc), ...walk(sdkSrc), ...walk(helloPluginDir)];

const RULES = [
  {
    name: "no-ambient-aurora",
    regex: /\boctarq-aurora\b|\bblur-3xl\b/,
    message: "decorative radial/blur glow is forbidden by the 3-surface design contract",
  },
  {
    name: "no-heavy-shadow",
    regex: /\bshadow-(?:xl|2xl)\b/,
    message: "heavy elevation shadows are forbidden; use crisp 1px borders or shadow-lg on overlays",
  },
  {
    name: "no-translate-hover-bounce",
    regex: /\bhover:-translate-y-/,
    message: "hover vertical bounce is forbidden; transition color/border instead",
  },
  {
    name: "no-native-dialog",
    regex: /\bwindow\.(alert|confirm|prompt)\s*\(|(?<!\.)\b(alert|confirm|prompt)\s*\(/,
    message: "native browser dialogs are forbidden; use toast or confirmDialog from @octarq/plugin-sdk",
  },
];

let errors = 0;

for (const full of files) {
  const rel = path.relative(repoRoot, full).replace(/\\/g, "/");
  const lines = fs.readFileSync(full, "utf8").split("\n");

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const trimmed = line.trim();
    if (trimmed.startsWith("//") || trimmed.startsWith("*")) continue;

    for (const rule of RULES) {
      if (rule.regex.test(line)) {
        console.error(`${rel}:${i + 1}: [${rule.name}] ${rule.message}`);
        errors++;
      }
    }

    if (/\bbackdrop-blur\b/.test(line) && !OVERLAY_BLUR_ALLOWLIST.has(rel)) {
      console.error(
        `${rel}:${i + 1}: [no-surface-blur] backdrop-blur is restricted to modal/drawer overlays`,
      );
      errors++;
    }
  }
}

if (errors > 0) {
  console.error(`\nFound ${errors} design-system violation(s).`);
  process.exit(1);
} else {
  console.log("Design-system surface & anti-pattern checks passed.");
}
