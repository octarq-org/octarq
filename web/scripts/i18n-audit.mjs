import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webDir = path.resolve(__dirname, "..");
const repoDir = path.resolve(webDir, "..");
const proDir = "/Volumes/PHD/code/octarq-pro";

let hasErrors = false;

// ---------------------------------------------------------------------------
// Part 1: i18n Key Completeness Audit
// ---------------------------------------------------------------------------

function loadTsExports(filePath) {
  const code = fs.readFileSync(filePath, "utf8");
  const result = ts.transpileModule(code, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  });
  const exports = {};
  const module = { exports };
  const fn = new Function("exports", "module", "require", result.outputText);
  fn(exports, module, () => ({}));
  return module.exports;
}

function getNestedKeys(obj, prefix = "") {
  let keys = [];
  if (!obj || typeof obj !== "object") return keys;
  for (const [k, v] of Object.entries(obj)) {
    const keyPath = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) {
      keys = keys.concat(getNestedKeys(v, keyPath));
    } else {
      keys.push(keyPath);
    }
  }
  return keys;
}

function checkDictionaryCompleteness() {
  console.log("=== Checking i18n Dictionary Key Completeness ===");
  const REQUIRED_WEB_LOCALES = ["zh", "es", "pt", "ja"];

  // 1. Check web/src/i18n/ (en.ts vs zh.ts, es.ts, pt.ts, ja.ts)
  const rootLocales = {};
  for (const lang of ["en", ...REQUIRED_WEB_LOCALES]) {
    const file = path.join(webDir, "src/i18n", `${lang}.ts`);
    if (fs.existsSync(file)) {
      const mod = loadTsExports(file);
      rootLocales[lang] = mod[lang] || {};
    }
  }
  if (rootLocales.en) {
    const enKeys = new Set(getNestedKeys(rootLocales.en));
    for (const lang of REQUIRED_WEB_LOCALES) {
      const langKeys = new Set(getNestedKeys(rootLocales[lang] || {}));
      const missing = [...enKeys].filter((k) => !langKeys.has(k));
      if (missing.length > 0) {
        console.error(`❌ [web/src/i18n/${lang}.ts] Missing ${missing.length} key(s) relative to en:`);
        missing.forEach((k) => console.error(`   - ${k}`));
        hasErrors = true;
      }
    }
  }

  // 2. Check web/src/i18n/pages/*.ts (Requires all 5 locales: en, zh, es, pt, ja)
  const pagesDir = path.join(webDir, "src/i18n/pages");
  if (fs.existsSync(pagesDir)) {
    const files = fs.readdirSync(pagesDir).filter((f) => f.endsWith(".ts") && f !== "index.ts");
    for (const file of files) {
      const filePath = path.join(pagesDir, file);
      const mod = loadTsExports(filePath);
      const nsName = Object.keys(mod)[0];
      const nsData = mod[nsName];
      if (nsData && nsData.en) {
        const enKeys = new Set(getNestedKeys(nsData.en));
        for (const lang of REQUIRED_WEB_LOCALES) {
          const langKeys = new Set(getNestedKeys(nsData[lang] || {}));
          const missing = [...enKeys].filter((k) => !langKeys.has(k));
          if (missing.length > 0) {
            console.error(`❌ [${path.relative(repoDir, filePath)} -> ${nsName}.${lang}] Missing ${missing.length} key(s) relative to en:`);
            missing.forEach((k) => console.error(`   - ${k}`));
            hasErrors = true;
          }
        }
      }
    }
  }

  // 3. Check Pro package i18n files (Checks all defined locales on each package against en)
  const proPackagesDir = path.join(proDir, "packages");
  if (fs.existsSync(proPackagesDir)) {
    const pkgDirs = fs.readdirSync(proPackagesDir);
    for (const pkg of pkgDirs) {
      const i18nFile = path.join(proPackagesDir, pkg, "src/i18n.ts");
      if (fs.existsSync(i18nFile)) {
        const mod = loadTsExports(i18nFile);
        for (const [nsName, nsData] of Object.entries(mod)) {
          if (nsData && typeof nsData === "object" && nsData.en) {
            const enKeys = new Set(getNestedKeys(nsData.en));
            const localesToCheck = Object.keys(nsData).filter((k) => k !== "en");
            for (const lang of localesToCheck) {
              const langKeys = new Set(getNestedKeys(nsData[lang] || {}));
              const missing = [...enKeys].filter((k) => !langKeys.has(k));
              if (missing.length > 0) {
                console.error(`❌ [${path.relative(proDir, i18nFile)} -> ${nsName}.${lang}] Missing ${missing.length} key(s) relative to en:`);
                missing.forEach((k) => console.error(`   - ${k}`));
                hasErrors = true;
              }
            }
          }
        }
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Part 2: Hardcoded User-Visible String Audit
// ---------------------------------------------------------------------------

// Exact match allowlist for non-translatable text with comments explaining why:
const ALLOWLIST_EXACT = new Set([
  // Product & brand names (should not be translated)
  "Octarq",
  "octarq",
  "octarq-pro",
  "Google",
  "GitHub",
  "Slack",
  "Stripe",
  "PostgreSQL",
  "SQLite",

  // Technical protocols, DNS record types & web standards
  "CNAME",
  "A / AAAA",
  "A (IPv4)",
  "AAAA (IPv6)",
  "UTM",
  "KB)",
  ".eml",
  "via",

  // Keyboard shortcuts & UI symbols / icons
  "⌘K",
  "esc",
  "Esc",
  "Ctrl",
  "Alt",
  "Shift",
  "Enter",
  "Space",
  "→",
  "—",
  "📝",
  "💡",
  "🔐",
  "loading…",
  "…",

  // Technical confirmation tokens & constant identifiers
  "DELETE MY DATA",
  "OCTARQ_PRO_LICENSE",
  "OCTARQ_ENDPOINT",
  "license.publicKeyB64",
  "plink_…",
  "metadata",
  "current",
  "Total",

  // Code example placeholders in UI inputs
  "Acme Production",
  "colleague@example.com",
  "n8n automation",
  "My Dev Team Slack",
  "My Dev Team Telegram",
  "go.example.com",
  "mail.example.com",
  "example.com",
  "promo2026",
  "q3-ads, product-hunt",
  "https://my-site.com/expired",
  "pricing\nlogin\nabout",
  "pricing&#10;login&#10;about",
  "admin\npostmaster",
  "admin&#10;postmaster",
  "smtp.mailgun.org",
  "noreply@domain.com",
  "deploy/cloudflare-email-worker.js",
  "user.login",
  "workspace",
  "octarq-client",
  "company.com",
  "Pro",
  "pro",
  "v1.2.0",
  "Acme Links",
  "Ov23li…",
  "Octarq Status Page • Powered by Octarq",
  "Octarq Status Page &bull; Powered by Octarq",

  // Accessibility aria-labels on generic icon buttons
  "Dismiss alert",
]);

// Pattern-based allowlist for non-translatable text formats:
const ALLOWLIST_PATTERNS = [
  /^https?:\/\//,                       // URLs
  /^data:/,                             // Data URIs
  /^\//,                                // Absolute paths
  /^@octarq/,                           // Package scopes
  /^\*[\.\w-]+$/,                       // Wildcard patterns like *.apps.googleusercontent.com
  /^e\.g\.\s/i,                         // Example hints in code placeholder (e.g. home.example.com)
  /^eyJ/,                               // Base64 JWT tokens
  /^-----BEGIN/,                        // PEM keys
  /^[A-Z0-9_]{3,}$/,                    // ENV variable names or all-caps IDs
  /^[a-z0-9_-]+\.[a-z]{2,}$/i,         // Domain examples like example.com
  /pricing.*login/i,                     // Multiline placeholder example in LinkSettings
  /admin.*postmaster/i,                  // Multiline placeholder example in MailSettings
];

function isAllowlisted(text) {
  const s = text.trim();
  if (!s) return true;
  // Pure punctuation, numbers, symbols
  if (/^[0-9\s\-_.:,;\/\\(){}\[\]*+="'"'"'!@#$%^&|<>?~`•·✕✓▾]+$/.test(s)) return true;
  if (ALLOWLIST_EXACT.has(s)) return true;
  for (const pat of ALLOWLIST_PATTERNS) {
    if (pat.test(s)) return true;
  }
  return false;
}

function getTsxFiles(dir) {
  let res = [];
  if (!fs.existsSync(dir)) return res;
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f);
    const stat = fs.statSync(p);
    if (stat.isDirectory()) {
      if (f !== "node_modules" && f !== "dist" && f !== ".git") {
        res = res.concat(getTsxFiles(p));
      }
    } else if (f.endsWith(".tsx")) {
      res.push(p);
    }
  }
  return res;
}

function checkHardcodedStrings() {
  console.log("\n=== Checking Hardcoded English Strings in TSX Files ===");

  const targetDirs = [
    path.join(webDir, "src"),
  ];
  if (fs.existsSync(path.join(proDir, "packages"))) {
    targetDirs.push(path.join(proDir, "packages"));
  }

  const tsxFiles = targetDirs.flatMap(getTsxFiles);

  for (const file of tsxFiles) {
    const code = fs.readFileSync(file, "utf8");
    const sf = ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);

    function getLine(pos) {
      return sf.getLineAndCharacterOfPosition(pos).line + 1;
    }

    function visit(node) {
      if (ts.isJsxText(node)) {
        const text = node.getText();
        if (!isAllowlisted(text)) {
          const displayPath = file.startsWith(proDir)
            ? path.relative(proDir, file)
            : path.relative(repoDir, file);
          console.error(`❌ ${displayPath}:${getLine(node.getStart())} [JSXText] "${text.trim()}"`);
          hasErrors = true;
        }
      } else if (ts.isJsxAttribute(node)) {
        const attrName = node.name.getText();
        if (["placeholder", "title", "aria-label"].includes(attrName) && node.initializer) {
          if (ts.isStringLiteral(node.initializer)) {
            const val = node.initializer.text;
            if (!isAllowlisted(val)) {
              const displayPath = file.startsWith(proDir)
                ? path.relative(proDir, file)
                : path.relative(repoDir, file);
              console.error(`❌ ${displayPath}:${getLine(node.getStart())} [${attrName}] "${val}"`);
              hasErrors = true;
            }
          }
        }
      }
      ts.forEachChild(node, visit);
    }

    visit(sf);
  }
}

// Run audits
checkDictionaryCompleteness();
checkHardcodedStrings();

if (hasErrors) {
  console.error("\n❌ i18n audit failed with errors above.");
  process.exit(1);
} else {
  console.log("\n✅ All i18n checks passed!");
  process.exit(0);
}
