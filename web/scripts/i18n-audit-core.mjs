// Core scanning logic for the hardcoded-string audit (i18n-audit.mjs Part 2),
// extracted into a module of its own so it can be unit-tested (src/test/
// i18nAuditCoverage.test.ts) without executing the whole CLI — importing
// i18n-audit.mjs would run every audit at module top level.
//
// The scanner is deliberately pure: input is a TSX source string, output is a
// list of hits. Reporting (console.error + hasErrors) stays in the CLI.

import * as ts from "typescript";

// Exact match allowlist for non-translatable text with comments explaining why:
export const ALLOWLIST_EXACT = new Set([
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

  // Legal / public pages (placeholder)
  "Terms of Service",
  "Privacy Policy",
]);

// Pattern-based allowlist for non-translatable text formats:
export const ALLOWLIST_PATTERNS = [
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

export function isAllowlisted(text) {
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

// JSX attributes whose value is copy rendered to the user — label, description,
// hint, alt, heading, emptyText, confirmLabel, cancelLabel, tooltip — as opposed
// to identifier attributes (id, name, type, variant, slot, …) which name a node
// or alter its behaviour and are never localised. The old hardcoded list
// ({placeholder, title, aria-label}) missed `label=` and friends, which is how
// whole settings forms shipped half-English while the audit stayed green.
export const COPY_ATTRIBUTES = new Set([
  "placeholder",
  "title",
  "aria-label",
  "label",
  "description",
  "hint",
  "alt",
  "heading",
  "emptyText",
  "confirmLabel",
  "cancelLabel",
  "tooltip",
]);

/**
 * Scan a TSX source string for hardcoded English user-visible strings.
 *
 * Returns an array of hits: { file, line, kind, attr, text }.
 *   kind: "JSXText"       — bare text between tags: <div>Copy</div>
 *   kind: "JSXExpression" — a string literal in an expression container: <div>{"Copy"}</div>
 *   kind: <attr name>     — a COPY_ATTRIBUTES attribute with a string literal value
 *
 * Intentionally NOT covered (high false-positive rate, left for later):
 *   - ternary / conditional expressions: {ok ? "Connected" : "Disconnected"}
 *     — the branches may be API values ("on"/"off"), not copy;
 *   - object/array literals: <Select options={[{ label: "DB (Default)" }]} />
 *     and `const TABS = [{ id: "a", label: "Settings" }]` — indistinguishable
 *     from data shapes without a component-type oracle. Both are noted here so
 *     the next person doesn't mistake the omission for a gap.
 */
export function scanTsxSource(code, file) {
  const sf = ts.createSourceFile(file, code, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const hits = [];

  function getLine(pos) {
    return sf.getLineAndCharacterOfPosition(pos).line + 1;
  }

  function visit(node) {
    if (ts.isJsxText(node)) {
      const text = node.getText();
      if (!isAllowlisted(text)) {
        hits.push({ file, line: getLine(node.getStart()), kind: "JSXText", attr: null, text: text.trim() });
      }
    } else if (ts.isJsxAttribute(node)) {
      const attrName = node.name.getText();
      if (COPY_ATTRIBUTES.has(attrName) && node.initializer) {
        if (ts.isStringLiteral(node.initializer) && !isAllowlisted(node.initializer.text)) {
          hits.push({ file, line: getLine(node.getStart()), kind: attrName, attr: attrName, text: node.initializer.text });
        }
      }
    } else if (ts.isJsxExpression(node)) {
      // <div>{"Copy"}</div> — a bare string literal in braces. Only the literal
      // shape; everything else in an expression container is either a variable
      // (already translated upstream) or conditional/object data (uncovered, see
      // the "Intentionally NOT covered" note on scanTsxSource).
      const expr = node.expression;
      if (expr && ts.isStringLiteral(expr) && !isAllowlisted(expr.text)) {
        hits.push({ file, line: getLine(node.getStart()), kind: "JSXExpression", attr: null, text: expr.text });
      }
    }
    ts.forEachChild(node, visit);
  }

  visit(sf);
  return hits;
}