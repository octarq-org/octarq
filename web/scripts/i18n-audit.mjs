import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as ts from "typescript";
import { scanTsxSource } from "./i18n-audit-core.mjs";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webDir = path.resolve(__dirname, "..");
const repoDir = path.resolve(webDir, "..");
const proDir = process.env.OCTARQ_PRO_DIR || path.resolve(repoDir, "../octarq-pro");

let hasErrors = false;


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

function checkPlaceholderParity(label, enObj, langObj, lang) {
  const en = getNestedStrings(enObj);
  const other = getNestedStrings(langObj || {});
  const bad = [];
  for (const [key, enText] of en) {
    if (!other.has(key)) continue;
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


const DEV_ONLY_DIRS = [path.join(webDir, "src", "dev")];

function isDevOnly(dir) {
  return DEV_ONLY_DIRS.some((dev) => dir === dev || dir.startsWith(dev + path.sep));
}

function getTsxFiles(dir) {
  let res = [];
  if (!fs.existsSync(dir)) return res;
  for (const f of fs.readdirSync(dir)) {
    const p = path.join(dir, f);
    const stat = fs.statSync(p);
    if (stat.isDirectory()) {
      if (f !== "node_modules" && f !== "dist" && f !== ".git" && !isDevOnly(p)) {
        res = res.concat(getTsxFiles(p));
      }
    } else if (f.endsWith(".tsx") && !/\.(test|spec)\.tsx?$/.test(f)) {
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
    for (const hit of scanTsxSource(code, file)) {
      const displayPath = file.startsWith(proDir)
        ? path.relative(proDir, file)
        : path.relative(repoDir, file);
      console.error(`❌ ${displayPath}:${hit.line} [${hit.kind}] "${hit.text}"`);
      hasErrors = true;
    }
  }
}


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

function findProp(objLit, name) {
  for (const prop of objLit.properties) {
    if (!ts.isPropertyAssignment(prop)) continue;
    const n = ts.isIdentifier(prop.name) || ts.isStringLiteral(prop.name) ? prop.name.text : null;
    if (n === name) return prop.initializer;
  }
  return null;
}

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
                const base = n === "_shared" ? "" : pluginName;
                if (n === "_shared" && ts.isObjectLiteralExpression(prop.initializer)) {
                  collectObjectKeys(prop.initializer, base, keys);
                } else if (ts.isObjectLiteralExpression(prop.initializer)) {
                  collectObjectKeys(prop.initializer, `${pluginName}.${n}`, keys);
                } else if (ts.isPropertyAccessExpression(prop.initializer)) {
                  const ref = dictIdents.get(prop.initializer.expression.getText());
                  const borrowed = ref ? findProp(ref, prop.initializer.name.text) : null;
                  if (borrowed && ts.isObjectLiteralExpression(borrowed)) {
                    collectObjectKeys(borrowed, `${pluginName}.${n}`, keys);
                  } else {
                    keys.add(`${pluginName}.${n}`);
                  }
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

function collectReferences(sourceFiles) {
  const staticRefs = [];
  const dynamicPrefixes = [];
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

  const roots = [path.join(webDir, "src"), path.join(repoDir, "packages"), path.join(repoDir, "server", "examples")];
  if (fs.existsSync(path.join(proDir, "packages"))) roots.push(path.join(proDir, "packages"));

  const files = roots.flatMap(getSourceFiles);
  const sourceFiles = parseAll(files);
  const dictIdents = indexDictionaryIdentifiers(sourceFiles);
  const { defined, origin } = definedKeysFor(sourceFiles, dictIdents);
  const { staticRefs, dynamicPrefixes, literals } = collectReferences(sourceFiles);

  const display = (f) => (f.startsWith(proDir) ? path.relative(proDir, f) : path.relative(repoDir, f));

  const unresolved = staticRefs.filter((r) => !defined.has(r.key));
  if (unresolved.length > 0) {
    console.error(`❌ ${unresolved.length} t() key(s) resolve to nothing (the key itself renders):`);
    for (const r of unresolved) {
      const suffixMatch = [...defined].filter((d) => d.endsWith(`.${r.key}`));
      const hint = suffixMatch.length === 1 ? `  → did you mean "${suffixMatch[0]}"?` : "";
      console.error(`   ${display(r.file)}:${r.line}  t("${r.key}")${hint}`);
    }
    hasErrors = true;
  }

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

  checkLocaleCoverage(sourceFiles, dictIdents);
  checkGoMenuI18n(defined);
}


const GO_MENU_ROOTS = [
  [repoDir, ["server/internal/api", "server/plugins", "server/examples"]],
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

const NON_HEADING_CATEGORIES = new Set(["footer", "resources", "settings"]);

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
