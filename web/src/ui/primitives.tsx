// Re-exports primitives from packages/plugin-sdk/src/ui via source path (to avoid import cycles), plus app-side Code and Guide components.
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
        className="flex w-full items-center justify-between px-3 py-2 text-left text-foreground/70 hover:bg-surface-hover"
        onClick={() => setShow((s) => !s)}
        aria-expanded={show}
      >
        <span className="flex items-center gap-2">
          <span>💡</span>
          {title}
        </span>
        <span className="text-muted-foreground" aria-hidden="true">{show ? "▾" : "▸"}</span>
      </button>
      {show && (
        <div className="space-y-2 border-t border-border px-3 py-3 text-muted-foreground animate-expand">
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
      className="cursor-pointer break-all rounded-lg bg-muted px-1.5 py-0.5 font-mono text-[12px] text-accent-fg hover:bg-surface-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400/60" /* ui-color-ok */
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
