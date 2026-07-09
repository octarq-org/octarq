// After the frontend-SDK extraction, the pure shared primitives live in the
// published package `@octarq-org/plugin-sdk/ui` (packages/plugin-sdk). This module
// re-exports them so the ~50 app files that import from "../ui" keep working
// unchanged, and it retains the two components that stay app-side:
//   - `Code`, which reads the app's i18n (`useTranslation`) for its copy label;
//   - `Guide`, kept here alongside `Code`.
//
// The package is imported by SOURCE PATH (not the `@octarq-org/plugin-sdk` name, which
// is aliased to the app facade) to keep the dependency pointing app → package
// and to avoid an import cycle through the facade's UI surface.
import { ReactNode, useState } from "react";
import { useTranslation } from "../i18n";

export * from "../../../packages/plugin-sdk/src/ui";

// ─── Guide ─────────────────────────────────────────────────────────────────

export function Guide({
  title,
  children,
  open = false,
}: {
  title: string;
  children: ReactNode;
  open?: boolean;
}) {
  const [show, setShow] = useState(open);
  return (
    <div className="glass mb-3 overflow-hidden rounded-2xl text-sm">
      <button
        className="flex w-full items-center justify-between px-3 py-2 text-left text-white/70 hover:bg-white/5"
        onClick={() => setShow((s) => !s)}
        aria-expanded={show}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-white/50" aria-hidden="true">{show ? "▾" : "▸"}</span>
      </button>
      {show && (
        <div className="space-y-2 border-t border-white/[0.06] px-3 py-3 text-white/55 animate-expand">
          {children}
        </div>
      )}
    </div>
  );
}

// ─── Code ──────────────────────────────────────────────────────────────────

export function Code({ children }: { children: string }) {
  const [copied, setCopied] = useState(false);
  const { t } = useTranslation();
  const copy = () => {
    navigator.clipboard.writeText(children);
    setCopied(true);
    setTimeout(() => setCopied(false), 1000);
  };
  return (
    <code
      role="button"
      tabIndex={0}
      aria-label={t("uiCommon.clickToCopy")}
      className="cursor-pointer break-all rounded-lg bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px] text-indigo-200 hover:bg-white/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60"
      title={t("uiCommon.clickToCopy")}
      onClick={copy}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          copy();
        }
      }}
    >
      {copied ? t("uiCommon.copied") : children}
    </code>
  );
}
