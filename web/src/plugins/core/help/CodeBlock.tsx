import { useRef, useState, type ReactNode } from "react";
import { Check, Copy } from "lucide-react";
import { useTranslation } from "../../../i18n";

export function CodeBlock({
  children,
  code,
  language,
}: {
  children: ReactNode;
  code: string;
  language?: string;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const codeRef = useRef<HTMLElement>(null);
  const label = language ? language.toUpperCase() : "CODE";

  const handleCopy = () => {
    const text = codeRef.current?.textContent ?? code;
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="code-block-wrapper">
      <div className="code-block-header">
        <span className="code-block-lang">{label}</span>
        <button
          type="button"
          className="code-block-copy-btn"
          onClick={handleCopy}
          title={copied ? t("help.copied", "Copied!") : t("help.copy_code", "Copy")}
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
          <span>{copied ? t("help.copied", "Copied!") : t("help.copy_code", "Copy")}</span>
        </button>
      </div>
      <pre>
        <code
          ref={codeRef}
          className={language ? `chroma language-${language}` : "chroma"}
        >
          {children}
        </code>
      </pre>
    </div>
  );
}
