import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as ts from "typescript";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webDir = path.resolve(__dirname, "..");
const repoDir = path.resolve(webDir, "..");
// The commercial repo, when it's checked out beside this one: its plugin
// packages carry UI that this dictionary feeds, so auditing them together
// catches a key that only one side knows about. Absent (CI, a third-party
// clone) every Pro check simply doesn't run — the path is probed, never
// required.
const proDir = process.env.OCTARQ_PRO_DIR || path.resolve(repoDir, "../octarq-pro");

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

// getNestedStrings is getNestedKeys but keeping the values, so a translation can
// be compared against its English source rather than merely counted.
function getNestedStrings(obj, prefix = "", out = new Map()) {
  if (!obj || typeof obj !== "object") return out;
  for (const [k, v] of Object.entries(obj)) {
    const keyPath = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) getNestedStrings(v, keyPath, out);
    else if (typeof v === "string") out.set(keyPath, v);
  }
  return out;
}

const PLACEHOLDER = /\{\{\s*([\w.]+)\s*\}\}/g;

function placeholdersIn(text) {
  return new Set([...text.matchAll(PLACEHOLDER)].map((m) => m[1]));
}

// A translation that drops (or invents) a {{placeholder}} still has the right
// key in the right locale, so key-parity above passes it. What ships is the
// literal "{{count}}" rendered on screen in that language only — visible to
// everyone except whoever added it. Interpolation is exactly what a translator
// or a bulk edit gets wrong, so it is worth comparing directly.
function checkPlaceholderParity(label, enObj, langObj, lang) {
  const en = getNestedStrings(enObj);
  const other = getNestedStrings(langObj || {});
  const bad = [];
  for (const [key, enText] of en) {
    if (!other.has(key)) continue; // absent is key-parity's job, reported there
    const want = placeholdersIn(enText);
    const got = placeholdersIn(other.get(key));
    const missing = [...want].filter((p) => !got.has(p));
    const extra = [...got].filter((p) => !want.has(p));
    if (missing.length || extra.length) bad.push({ key, missing, extra });
  }
  if (bad.length) {
    console.error(`❌ [${label} -> ${lang}] ${bad.length} translation(s) with mismatched placeholders:`);
    for (const b of bad) {
      const parts = [];
      if (b.missing.length) parts.push(`missing {{${b.missing.join("}}, {{")}}}`);
      if (b.extra.length) parts.push(`unexpected {{${b.extra.join("}}, {{")}}}`);
      console.error(`   - ${b.key}: ${parts.join("; ")}`);
    }
    hasErrors = true;
  }
}

function loadUntranslatedAllowlist() {
  const allowlistFile = path.join(webDir, "src/i18n/untranslated-allowlist.ts");
  if (!fs.existsSync(allowlistFile)) return new Set();
  const mod = loadTsExports(allowlistFile);
  const setOrArray = mod.UNTRANSLATED_ALLOWLIST;
  if (setOrArray instanceof Set) return setOrArray;
  if (Array.isArray(setOrArray)) return new Set(setOrArray);
  return new Set();
}

function checkUntranslatedValues(label, enObj, langObj, lang, allowlist, nsPrefix = "") {
  const en = getNestedStrings(enObj);
  const other = getNestedStrings(langObj || {});
  const bad = [];
  for (const [key, enText] of en) {
    if (!other.has(key)) continue;
    const fullKey = nsPrefix ? `${nsPrefix}.${key}` : key;
    const langText = other.get(key);
    if (langText === enText && !allowlist.has(fullKey)) {
      bad.push({ key: fullKey, text: langText });
    }
  }
  if (bad.length > 0) {
    console.error(`❌ [${label} -> ${lang}] ${bad.length} untranslated key(s) identical to en (not in allowlist):`);
    for (const b of bad) {
      console.error(`   - ${b.key}: "${b.text}"`);
    }
    hasErrors = true;
  }
}

function checkDictionaryCompleteness() {
  console.log("=== Checking i18n Dictionary Key Completeness ===");
  const REQUIRED_WEB_LOCALES = ["zh", "es", "pt", "ja"];
  const allowlist = loadUntranslatedAllowlist();

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
      checkPlaceholderParity(`web/src/i18n/${lang}.ts`, rootLocales.en, rootLocales[lang], lang);
      checkUntranslatedValues(`web/src/i18n/${lang}.ts`, rootLocales.en, rootLocales[lang], lang, allowlist);
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
          checkPlaceholderParity(`${path.relative(repoDir, filePath)} -> ${nsName}`, nsData.en, nsData[lang], lang);
          checkUntranslatedValues(`${path.relative(repoDir, filePath)} -> ${nsName}`, nsData.en, nsData[lang], lang, allowlist, nsName);
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
              checkPlaceholderParity(`${path.relative(proDir, i18nFile)} -> ${nsName}`, nsData.en, nsData[lang], lang);
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
  // Example workspace slug. It is a URL label typed verbatim, so localising it
  // would suggest the address itself changes with the interface language.
  "acme",
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
  // ACME account email placeholder (plugin-infra certificates page) and the S3
  // key-prefix placeholder next to it — both are input examples, not copy.
  "admin@example.com",
  "folder/",
  // Object-key example in the S3 upload dialog. A path is a path in every
  // language; translating "file" here would suggest the key itself is localised.
  "folder/file.png",
  // S3 region and bucket examples in the mail-storage card (octarq-pro). A
  // region code is an AWS identifier and a bucket name is a global DNS label —
  // both are typed verbatim by the operator, so localising them would suggest
  // the value itself changes with the interface language. The sibling endpoint
  // placeholder on that card is already exempt for being a URL.
  "us-east-1",
  "octarq-mail-blobs",
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
  /^(?:&[a-z]+;|\s)+$/,                 // Bare HTML entities (&mdash; &bull; &larr;) — typography, not copy
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
    } else if (f.endsWith(".tsx") && !/\.(test|spec)\.tsx?$/.test(f)) {
      // Test files are fixtures, not shipped UI — the key-resolution walk below
      // already skips them; this walk drifted and did not.
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

// ---------------------------------------------------------------------------
// Part 3: Key Resolution Audit — references vs definitions
// ---------------------------------------------------------------------------
//
// Parts 1 and 2 check locale parity and untranslated literals. Neither catches
// the two failure modes that actually shipped:
//
//   - `t("settings.pluginInUse")` with that key defined in NO locale, not even
//     en. Parity is perfect (all five are equally missing), nothing is
//     hardcoded, and the UI renders the literal string "settings.pluginInUse".
//   - `t("pageTitle")` where the key really lives at `storage.pageTitle`,
//     because a UIPlugin's dictionary is nested under its `name`. Same silent
//     result: the key itself renders as the label.
//
// Both are invisible to a reader (t() falls back to the key, which looks like a
// plausible identifier, not an error) and invisible to tsc (t takes a string).
// So resolve every static key against the dictionary the runtime actually
// builds, and flag the reverse too: keys defined and translated five times over
// that nothing references.

// Mirrors the runtime merge in packages/plugin-sdk/src/i18n and
// contract/registry.ts. Namespacing rules, which differ by source:
//   - web/src/i18n/en.ts        → keys sit at the root
//   - web/src/i18n/pages/x.ts   → nested under the export name ("settings.save")
//   - a UIPlugin's `i18n`       → nested under the plugin's `name`, EXCEPT
//                                 `_shared`, which merges at the root
function collectObjectKeys(node, prefix, out) {
  if (!node || !ts.isObjectLiteralExpression(node)) return;
  for (const prop of node.properties) {
    if (!ts.isPropertyAssignment(prop)) continue;
    const name = ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name) ? prop.name.text : null;
    if (name === null) continue;
    const keyPath = prefix ? `${prefix}.${name}` : name;
    if (ts.isObjectLiteralExpression(prop.initializer)) {
      collectObjectKeys(prop.initializer, keyPath, out);
    } else {
      out.add(keyPath);
    }
  }
}

// findProp returns the initializer of a named property on an object literal.
function findProp(objLit, name) {
  for (const prop of objLit.properties) {
    if (!ts.isPropertyAssignment(prop)) continue;
    const n = ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name) ? prop.name.text : null;
    if (n === name) return prop.initializer;
  }
  return null;
}

// Dictionary literals reached by name (`i18n: settings`) rather than written
// inline, so a UIPlugin that imports its dictionary still resolves. Maps the
// local identifier to its object literal, per file and across the i18n.ts files
// a plugin package imports from.
function indexDictionaryIdentifiers(sourceFiles) {
  const byName = new Map();
  for (const sf of sourceFiles) {
    const visit = (node) => {
      if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.initializer) {
        if (ts.isObjectLiteralExpression(node.initializer)) {
          byName.set(node.name.text, node.initializer);
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(sf);
  }
  return byName;
}

function definedKeysFor(sourceFiles, dictIdents) {
  const defined = new Set();
  // Where each key was defined, for the "never referenced" report.
  const origin = new Map();

  const record = (keys, file) => {
    for (const k of keys) {
      defined.add(k);
      if (!origin.has(k)) origin.set(k, file);
    }
  };

  for (const sf of sourceFiles) {
    const rel = sf.fileName;
    const isShellDict = /web\/src\/i18n\/(en)\.ts$/.test(rel);
    const isPageDict = /web\/src\/i18n\/pages\/[^/]+\.ts$/.test(rel) && !rel.endsWith("index.ts");

    const visit = (node) => {
      // Two shapes, and they nest differently:
      //   web/src/i18n/en.ts       → `export const en = { common: {...}, … }`
      //     the object IS the root dictionary, no per-locale wrapper.
      //   web/src/i18n/pages/x.ts  → `export const x = { en: {...}, zh: {...} }`
      //     the en block hangs off the export name.
      if (
        (isShellDict || isPageDict) &&
        ts.isVariableDeclaration(node) &&
        ts.isIdentifier(node.name) &&
        node.initializer &&
        ts.isObjectLiteralExpression(node.initializer)
      ) {
        if (isShellDict) {
          const keys = new Set();
          collectObjectKeys(node.initializer, "", keys);
          record(keys, rel);
        } else {
          const en = findProp(node.initializer, "en");
          if (en && ts.isObjectLiteralExpression(en)) {
            const keys = new Set();
            collectObjectKeys(en, node.name.text, keys);
            record(keys, rel);
          }
        }
      }

      // A UIPlugin object literal: has both `name` (string) and `i18n`.
      if (ts.isObjectLiteralExpression(node)) {
        const nameProp = findProp(node, "name");
        const i18nProp = findProp(node, "i18n");
        if (nameProp && i18nProp && ts.isStringLiteral(nameProp)) {
          const pluginName = nameProp.text;
          let dictLit = null;
          if (ts.isObjectLiteralExpression(i18nProp)) dictLit = i18nProp;
          else if (ts.isIdentifier(i18nProp)) dictLit = dictIdents.get(i18nProp.text) ?? null;
          if (dictLit) {
            const en = findProp(dictLit, "en");
            if (en && ts.isObjectLiteralExpression(en)) {
              const keys = new Set();
              for (const prop of en.properties) {
                if (!ts.isPropertyAssignment(prop)) continue;
                const n = ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name) ? prop.name.text : null;
                if (n === null) continue;
                // `_shared` merges at the root; everything else hangs off the
                // plugin name.
                const base = n === "_shared" ? "" : pluginName;
                if (n === "_shared" && ts.isObjectLiteralExpression(prop.initializer)) {
                  collectObjectKeys(prop.initializer, base, keys);
                } else if (ts.isObjectLiteralExpression(prop.initializer)) {
                  collectObjectKeys(prop.initializer, `${pluginName}.${n}`, keys);
                } else {
                  keys.add(`${pluginName}.${n}`);
                }
              }
              record(keys, rel);
            }
          }
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(sf);
  }
  return { defined, origin };
}

// Every t("…") / t(`…`) call. Static keys resolve exactly; a key built by
// concatenation or interpolation (t(`help.group.${g}`), t("settings.pluginDesc." + k))
// can only contribute a PREFIX — the suffix is runtime data, so any defined key
// under that prefix counts as referenced and none of them can be checked.
function collectReferences(sourceFiles) {
  const staticRefs = [];
  const dynamicPrefixes = [];
  // Every string literal in the tree, t() call or not. A key is routinely
  // passed around before it is translated — auth.tsx hands
  // "settings.clearGoogleSecret" to a helper that calls t(confirmKey), and the
  // plugin manager stores "settings.pluginCategory.ai" in a lookup table. Those
  // keys are live, but no t("…") mentions them, so without this they'd read as
  // dead. Only used to suppress the unreferenced report, never to resolve.
  const literals = new Set();

  for (const sf of sourceFiles) {
    const visit = (node) => {
      if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
        literals.add(node.text);
      }
      if (ts.isCallExpression(node)) {
        const callee = node.expression;
        const isT =
          (ts.isIdentifier(callee) && callee.text === "t") ||
          (ts.isPropertyAccessExpression(callee) && callee.name.text === "t");
        const arg = node.arguments[0];
        if (isT && arg) {
          const line = sf.getLineAndCharacterOfPosition(node.getStart()).line + 1;
          if (ts.isStringLiteral(arg) || ts.isNoSubstitutionTemplateLiteral(arg)) {
            staticRefs.push({ key: arg.text, file: sf.fileName, line });
          } else if (ts.isTemplateExpression(arg)) {
            dynamicPrefixes.push(arg.head.text);
          } else if (ts.isBinaryExpression(arg) && arg.operatorToken.kind === ts.SyntaxKind.PlusToken) {
            let left = arg.left;
            while (ts.isBinaryExpression(left)) left = left.left;
            if (ts.isStringLiteral(left)) dynamicPrefixes.push(left.text);
          }
          // t(someVariable) contributes nothing either way: it can't be
          // resolved, and it can't be used to prove a key is dead. Such keys
          // surface as false positives in the unreferenced report, which is why
          // that report is a warning and not a failure.
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(sf);
  }
  return { staticRefs, dynamicPrefixes, literals };
}

function parseAll(files) {
  return files.map((f) =>
    ts.createSourceFile(f, fs.readFileSync(f, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX),
  );
}

function getSourceFiles(dir) {
  let res = [];
  if (!fs.existsSync(dir)) return res;
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f);
    const stat = fs.statSync(p);
    if (stat.isDirectory()) {
      if (f !== "node_modules" && f !== "dist" && f !== ".git") res = res.concat(getSourceFiles(p));
    } else if (/\.(ts|tsx)$/.test(f) && !/\.(test|spec)\.tsx?$/.test(f)) {
      res.push(p);
    }
  }
  return res;
}

function checkKeyResolution() {
  console.log("\n=== Checking i18n Key Resolution (references vs definitions) ===");

  // packages/ on this side holds the plugin SDK, whose own components call t()
  // for the shared uiCommon keys — leave it out and those read as dead.
  // examples/ is in scope because it is the plugin authors copy: its dictionary
  // has to satisfy the same rules the audit enforces everywhere else, or the
  // reference implementation teaches the gap.
  const roots = [path.join(webDir, "src"), path.join(repoDir, "packages"), path.join(repoDir, "examples")];
  if (fs.existsSync(path.join(proDir, "packages"))) roots.push(path.join(proDir, "packages"));

  const files = roots.flatMap(getSourceFiles);
  const sourceFiles = parseAll(files);
  const dictIdents = indexDictionaryIdentifiers(sourceFiles);
  const { defined, origin } = definedKeysFor(sourceFiles, dictIdents);
  const { staticRefs, dynamicPrefixes, literals } = collectReferences(sourceFiles);

  const display = (f) => (f.startsWith(proDir) ? path.relative(proDir, f) : path.relative(repoDir, f));

  // 3a. Referenced but never defined — this is a bug on screen, so it fails.
  const unresolved = staticRefs.filter((r) => !defined.has(r.key));
  if (unresolved.length > 0) {
    console.error(`❌ ${unresolved.length} t() key(s) resolve to nothing (the key itself renders):`);
    for (const r of unresolved) {
      // Point at the likely namespace when one exists, since the usual cause is
      // a missing plugin-name prefix rather than a truly absent key.
      const suffixMatch = [...defined].filter((d) => d.endsWith(`.${r.key}`));
      const hint = suffixMatch.length === 1 ? `  → did you mean "${suffixMatch[0]}"?` : "";
      console.error(`   ${display(r.file)}:${r.line}  t("${r.key}")${hint}`);
    }
    hasErrors = true;
  }

  // 3b. Defined but never referenced — dead weight, translated five times over.
  // A warning, not a failure: t(variable) call sites are unresolvable, so this
  // list can name a key that is genuinely used.
  const referenced = new Set(staticRefs.map((r) => r.key));
  const orphans = [...defined].filter(
    (k) => !referenced.has(k) && !literals.has(k) && !dynamicPrefixes.some((p) => k.startsWith(p)),
  );
  if (orphans.length > 0) {
    console.warn(`\n⚠️  ${orphans.length} defined key(s) with no static reference (candidates for deletion):`);
    for (const k of orphans.sort()) {
      console.warn(`   ${k}   [${display(origin.get(k))}]`);
    }
  }

  if (unresolved.length === 0) {
    console.log(`✅ all ${staticRefs.length} static t() keys resolve`);
  }

  // Reuse the parse — walking every source file twice to answer a second
  // question about the same trees is pure waste.
  checkLocaleCoverage(sourceFiles, dictIdents);
  checkGoMenuI18n(defined);
}

// ---------------------------------------------------------------------------
// Part 5: Go menu i18n — every sidebar entry the backend announces
// ---------------------------------------------------------------------------
//
// Sidebar labels come from the Go half's MenuProvider, and the shell renders
// them as t(`nav.${id}`, item.label) / t(`groups.${category}`, label). The
// second argument means a missing key does NOT show the key on screen — it
// shows the English label the Go source declared. That is why this gap is
// invisible: the sidebar looks fine in every language, it is simply half
// untranslated, and nothing in the pipeline complains.
//
// navI18n.test.ts asserts the same property, but it reads Go sources through
// import.meta.glob, which cannot escape the Vite root — so it only ever saw
// THIS repo's menus. Every Pro menu (15 of them) was unchecked, and two were in
// fact untranslated. This check runs in the same process as the Pro dictionary
// scan above, so it can see both sides whenever both are checked out.

const GO_MENU_ROOTS = [
  [repoDir, ["internal/api", "plugins", "examples"]],
  [proDir, ["modules"]],
];

function goSourceFiles(root, subdirs) {
  const out = [];
  const walk = (dir) => {
    if (!fs.existsSync(dir)) return;
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === "node_modules" || entry.name === "testdata") continue;
        walk(full);
      } else if (entry.name.endsWith(".go") && !entry.name.endsWith("_test.go")) {
        out.push(full);
      }
    }
  };
  for (const sub of subdirs) walk(path.join(root, sub));
  return out;
}

// A MenuItem literal, e.g. {ID: "vps", Label: "Servers", Category: "Hosting", …}.
// Category is optional in the shape but present on every real entry; an entry
// without one is skipped rather than guessed at.
const GO_MENU_LITERAL = /\{ID:\s*"([^"]+)"([^}]*)\}/g;
const GO_MENU_CATEGORY = /Category:\s*"([^"]*)"/;

function collectGoMenus() {
  const menus = [];
  for (const [root, subdirs] of GO_MENU_ROOTS) {
    for (const file of goSourceFiles(root, subdirs)) {
      const code = fs.readFileSync(file, "utf8");
      for (const m of code.matchAll(GO_MENU_LITERAL)) {
        const cat = GO_MENU_CATEGORY.exec(m[2]);
        menus.push({ id: m[1], category: cat ? cat[1] : "", file });
      }
    }
  }
  return menus;
}

// Categories that never render as a group heading, so they need no groups.* key:
//   footer / resources → the rail footer, which draws no heading at all
//   settings           → App.tsx remaps a generic "settings" category to the
//                        Workspace group inside the Settings area, so the word
//                        "Settings" is never itself a heading
// Kept deliberately short: this mirrors placement logic that lives in
// areas.tsx/App.tsx, and every entry here is a chance for the two to drift.
const NON_HEADING_CATEGORIES = new Set(["footer", "resources", "settings"]);

// Every icon key a Go source names, from either place one can appear: a
// MenuItem's Icon (sidebar) or plugin.Info's Icon (feature card). A key that
// isn't in the PLUGIN_ICONS table doesn't fail — the shell renders the string
// itself, so "book" appeared as the word "book" next to Help in the sidebar for
// as long as it took someone to notice.
const GO_ICON = /\bIcon:\s*"([^"]*)"/g;

function collectGoIcons() {
  const icons = [];
  for (const [root, subdirs] of GO_MENU_ROOTS) {
    for (const file of goSourceFiles(root, subdirs)) {
      const code = fs.readFileSync(file, "utf8");
      for (const m of code.matchAll(GO_ICON)) {
        if (m[1]) icons.push({ key: m[1], file });
      }
    }
  }
  return icons;
}

// The keys of the PLUGIN_ICONS map in web/src/shell/areas.tsx, read from the
// source rather than restated here — a copy in this script would drift from the
// table and start passing icons the shell can't resolve.
function registeredIconKeys() {
  const file = path.join(webDir, "src/shell/areas.tsx");
  const sf = ts.createSourceFile(file, fs.readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const keys = new Set();
  const visit = (node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "PLUGIN_ICONS" &&
      node.initializer
    ) {
      // The map may be written as `{...} as const` / with a type assertion.
      let init = node.initializer;
      while (ts.isAsExpression(init) || ts.isTypeAssertionExpression?.(init)) init = init.expression;
      if (ts.isObjectLiteralExpression(init)) {
        for (const prop of init.properties) {
          if (!prop.name) continue;
          if (ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name)) keys.add(prop.name.text);
        }
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return keys;
}

function checkGoMenuIcons() {
  const registered = registeredIconKeys();
  if (registered.size === 0) {
    console.error("❌ parsed no PLUGIN_ICONS entries from web/src/shell/areas.tsx — the table's shape probably changed");
    hasErrors = true;
    return;
  }

  const unknown = [];
  const seen = new Set();
  for (const { key, file } of collectGoIcons()) {
    if (seen.has(key)) continue;
    seen.add(key);
    if (!registered.has(key)) {
      const where = file.startsWith(proDir) ? `octarq-pro/${path.relative(proDir, file)}` : path.relative(repoDir, file);
      unknown.push({ key, where });
    }
  }

  if (unknown.length === 0) {
    console.log(`✅ all ${seen.size} Go icon keys resolve against PLUGIN_ICONS`);
    return;
  }
  console.error(`❌ ${unknown.length} Go icon key(s) are not in PLUGIN_ICONS (the shell renders the string as text):`);
  for (const u of unknown) console.error(`   ${u.key.padEnd(28)} declared in ${u.where}`);
  hasErrors = true;
}

function checkGoMenuI18n(defined) {
  console.log("\n=== Checking Go Menu i18n (sidebar ids & group headings) ===");

  const menus = collectGoMenus();
  // An empty parse would make this pass vacuously — which is worse than failing,
  // because the check would go quiet exactly when the Go MenuItem shape changed.
  if (menus.length < 4) {
    console.error(`❌ parsed only ${menus.length} Go menu entries — the MenuItem literal shape probably changed`);
    hasErrors = true;
    return;
  }

  const missing = [];
  const seenIds = new Set();
  const seenCats = new Set();

  for (const { id, category, file } of menus) {
    const where = file.startsWith(proDir) ? `octarq-pro/${path.relative(proDir, file)}` : path.relative(repoDir, file);
    if (!seenIds.has(id)) {
      seenIds.add(id);
      if (!defined.has(`nav.${id}`)) missing.push({ key: `nav.${id}`, where });
    }
    const cat = category.toLowerCase();
    if (category && !NON_HEADING_CATEGORIES.has(cat) && !seenCats.has(category)) {
      seenCats.add(category);
      if (!defined.has(`groups.${category}`)) missing.push({ key: `groups.${category}`, where });
    }
  }

  if (missing.length === 0) {
    console.log(`✅ all ${seenIds.size} Go menu ids and ${seenCats.size} group headings have translations`);
  } else {
    console.error(`❌ ${missing.length} Go menu label(s) have no translation key (the sidebar silently stays English):`);
    for (const m of missing) console.error(`   ${m.key.padEnd(28)} declared in ${m.where}`);
    hasErrors = true;
  }

  checkGoMenuIcons();
}

// ---------------------------------------------------------------------------
// Part 4: Locale Coverage — which dictionaries don't ship all five languages
// ---------------------------------------------------------------------------
//
// Part 1 compares each locale a dictionary DECLARES against its en block, so a
// dictionary that simply omits es/pt/ja passes: there is nothing to compare.
// That is exactly how the links/mail/dns plugins came to serve ~90% English to
// Spanish, Portuguese and Japanese users while the audit stayed green — the
// runtime falls back to en per key, so nothing looks broken, it just isn't
// translated.
//
// Enforced. It was reported-only while the five-locale commitment was still an
// open question; the answer is yes, ship all five, and every dictionary now
// does. Anything less is a regression, and a silent one — the runtime falls
// back per key, so an untranslated page looks fine to whoever added it.
function checkLocaleCoverage(sourceFiles, dictIdents) {
  console.log("\n=== Checking Locale Coverage (plugin dictionaries) ===");
  const REQUIRED = ["zh", "es", "pt", "ja"];
  const rows = [];

  for (const sf of sourceFiles) {
    const visit = (node) => {
      if (ts.isObjectLiteralExpression(node)) {
        const nameProp = findProp(node, "name");
        const i18nProp = findProp(node, "i18n");
        if (nameProp && i18nProp && ts.isStringLiteral(nameProp)) {
          let dictLit = null;
          if (ts.isObjectLiteralExpression(i18nProp)) dictLit = i18nProp;
          else if (ts.isIdentifier(i18nProp)) dictLit = dictIdents.get(i18nProp.text) ?? null;
          const en = dictLit && findProp(dictLit, "en");
          if (en && ts.isObjectLiteralExpression(en)) {
            const enKeys = new Set();
            collectObjectKeys(en, "", enKeys);
            const gaps = [];
            for (const lang of REQUIRED) {
              const block = findProp(dictLit, lang);
              if (!block || !ts.isObjectLiteralExpression(block)) {
                gaps.push(`${lang} (absent)`);
                continue;
              }
              const langKeys = new Set();
              collectObjectKeys(block, "", langKeys);
              const missing = [...enKeys].filter((k) => !langKeys.has(k)).length;
              if (missing > 0) gaps.push(`${lang} −${missing}`);
            }
            if (gaps.length) rows.push({ plugin: nameProp.text, total: enKeys.size, gaps });
          }
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(sf);
  }

  if (rows.length === 0) {
    console.log("✅ every plugin dictionary ships all five locales in full");
    return;
  }
  console.error(`❌ ${rows.length} plugin dictionar(ies) are not fully translated:`);
  for (const r of rows.sort((a, b) => a.plugin.localeCompare(b.plugin))) {
    console.error(`   ${r.plugin.padEnd(16)} ${String(r.total).padStart(4)} en keys   ${r.gaps.join(", ")}`);
  }
  hasErrors = true;
}

// Run audits
checkDictionaryCompleteness();
checkHardcodedStrings();
checkKeyResolution();

if (hasErrors) {
  console.error("\n❌ i18n audit failed with errors above.");
  process.exit(1);
} else {
  console.log("\n✅ All i18n checks passed!");
  process.exit(0);
}
