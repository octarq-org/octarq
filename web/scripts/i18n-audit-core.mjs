
import * as ts from "typescript";

export const ALLOWLIST_EXACT = new Set([
  "Octarq",
  "octarq",
  "octarq-pro",
  "Google",
  "GitHub",
  "Slack",
  "Stripe",
  "PostgreSQL",
  "SQLite",

  "CNAME",
  "A / AAAA",
  "A (IPv4)",
  "AAAA (IPv6)",
  "UTM",
  "KB)",
  ".eml",
  "via",

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

  "DELETE MY DATA",
  "DELETE MY ACCOUNT",
  "OCTARQ_PRO_LICENSE",
  "OCTARQ_ENDPOINT",
  "license.publicKeyB64",
  "plink_…",
  "metadata",
  "current",
  "Total",

  "Acme Production",
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
  "admin@example.com",
  "alerts@example.com",
  "folder/",
  "folder/file.png",
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
  "v2",
  "HMAC-SHA256",
  "HMAC-SHA256 Signatures",
  "Acme Links",
  "Ov23li…",
  "Octarq Status Page • Powered by Octarq",
  "Octarq Status Page &bull; Powered by Octarq",
  "OCTARQ PRO ENTERPRISE BRIEFING",
  "sec",
  "x",

  "Dismiss alert",

  "Terms of Service",
  "Privacy Policy",
]);

export const ALLOWLIST_PATTERNS = [
  /^(?:&[a-z]+;|\s)+$/,
  /^https?:\/\//,
  /^data:/,
  /^\//,
  /^@octarq/,
  /^\*[\.\w-]+$/,
  /^e\.g\.\s/i,
  /^eyJ/,
  /^-----BEGIN/,
  /^[A-Z0-9_]{3,}$/,
  /^[a-z0-9_-]+\.[a-z]{2,}$/i,
  /pricing.*login/i,
  /admin.*postmaster/i,
];

export function isAllowlisted(text) {
  const s = text.trim();
  if (!s) return true;
  if (/^[0-9\s\-_.:,;\/\\(){}\[\]*+="'"'"'!@#$%^&|<>?~`•·✕✓▾]+$/.test(s)) return true;
  if (ALLOWLIST_EXACT.has(s)) return true;
  for (const pat of ALLOWLIST_PATTERNS) {
    if (pat.test(s)) return true;
  }
  return false;
}

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