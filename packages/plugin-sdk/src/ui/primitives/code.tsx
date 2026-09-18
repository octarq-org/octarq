import { useState } from "react";
import { useTranslation } from "../../i18n";

// Click-to-copy affordance for machine values (hostnames, DKIM records, API
// keys). Moved here from the host app so a plugin package can render one; it
// reads the SDK's own i18n context, so the uiCommon.* keys below come from the
// host dictionary like every other SDK component.
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
      className="cursor-pointer break-all rounded-lg bg-muted px-1.5 py-0.5 font-mono text-[12px] text-accent-fg hover:bg-surface-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
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
