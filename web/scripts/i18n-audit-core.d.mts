// Type declarations for i18n-audit-core.mjs, consumed by the vitest guard tests
// in src/test/i18nAuditCoverage.test.ts. The resolver (allowlists, isAllowlisted,
// scanTsxSource) is plain ESM; these describe its exports so tsc can type-check
// the tests that import it.

export type ScanHit = {
  file: string;
  line: number;
  /** "JSXText", "JSXExpression", or a COPY_ATTRIBUTES name (label, alt, …). */
  kind: string;
  /** The attribute name when kind is an attribute, otherwise null. */
  attr: string | null;
  text: string;
};

export declare const ALLOWLIST_EXACT: Set<string>;
export declare const ALLOWLIST_PATTERNS: RegExp[];
export declare const COPY_ATTRIBUTES: Set<string>;
export declare function isAllowlisted(text: string): boolean;
export declare function scanTsxSource(code: string, file: string): ScanHit[];