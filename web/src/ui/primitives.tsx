// After the frontend-SDK extraction, the pure shared primitives live in the
// published package `@led/plugin-sdk/ui` (packages/plugin-sdk). This module
// re-exports them so the ~50 app files that import from "../ui" keep working
// unchanged, and it retains the two components that stay app-side:
//   - `Code`, which reads the app's i18n (`useTranslation`) for its copy label;
//   - `Guide`, kept here alongside `Code`.
//
// The package is imported by SOURCE PATH (not the `@led/plugin-sdk` name, which
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
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-white/35">{show ? "▾" : "▸"}</span>
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
  return (
    <code
      className="cursor-pointer break-all rounded-lg bg-white/[0.06] px-1.5 py-0.5 font-mono text-[12px] text-indigo-200 hover:bg-white/10"
      title={t("uiCommon.clickToCopy")}
      onClick={() => {
        navigator.clipboard.writeText(children);
        setCopied(true);
        setTimeout(() => setCopied(false), 1000);
      }}
    >
      {copied ? t("uiCommon.copied") : children}
    </code>
  );
}
