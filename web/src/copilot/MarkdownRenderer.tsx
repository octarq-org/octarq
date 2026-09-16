import React from "react";
import { cn } from "../ui";

interface MarkdownRendererProps {
  content: string;
  className?: string;
}

export function MarkdownRenderer({ content, className }: MarkdownRendererProps) {
  if (!content) return null;

  // Split by code blocks first
  const parts = content.split(/(```[\s\S]*?```)/g);

  return (
    <div className={cn("space-y-2 text-sm leading-relaxed break-words", className)}>
      {parts.map((part, pIdx) => {
        if (part.startsWith("```") && part.endsWith("```")) {
          const lines = part.slice(3, -3).trim().split("\n");
          let language = "";
          let codeLines = lines;
          if (lines.length > 0 && !lines[0].includes(" ") && lines[0].length < 20) {
            language = lines[0].trim();
            codeLines = lines.slice(1);
          }
          const codeText = codeLines.join("\n");
          return (
            <div key={pIdx} className="my-2 overflow-hidden rounded-xl border border-border bg-muted/60 font-mono text-xs">
              {language && (
                <div className="border-b border-border/60 bg-muted/90 px-3 py-1 text-[11px] font-medium text-muted-foreground uppercase">
                  {language}
                </div>
              )}
              <pre className="overflow-x-auto p-3 text-foreground/90">
                <code>{codeText}</code>
              </pre>
            </div>
          );
        }

        // Regular markdown text paragraphs and lines
        const paragraphs = part.split(/\n\n+/);
        return (
          <React.Fragment key={pIdx}>
            {paragraphs.map((para, pIndex) => {
              const trimmed = para.trim();
              if (!trimmed) return null;

              // Blockquote
              if (trimmed.startsWith(">")) {
                const quoteText = trimmed
                  .split("\n")
                  .map((l) => l.replace(/^>\s?/, ""))
                  .join(" ");
                return (
                  <blockquote
                    key={pIndex}
                    className="border-l-2 border-primary/50 bg-primary/5 pl-3 py-1 text-xs italic text-muted-foreground my-1.5 rounded-r"
                  >
                    {renderInline(quoteText)}
                  </blockquote>
                );
              }

              // Bullet list
              if (trimmed.split("\n").every((l) => l.trim().startsWith("- ") || l.trim().startsWith("* "))) {
                const items = trimmed.split("\n").map((l) => l.trim().replace(/^[-*]\s+/, ""));
                return (
                  <ul key={pIndex} className="list-disc list-inside space-y-1 my-1.5 pl-1 text-xs sm:text-sm">
                    {items.map((item, i) => (
                      <li key={i}>{renderInline(item)}</li>
                    ))}
                  </ul>
                );
              }

              // Numbered list
              if (trimmed.split("\n").every((l) => /^\d+\.\s+/.test(l.trim()))) {
                const items = trimmed.split("\n").map((l) => l.trim().replace(/^\d+\.\s+/, ""));
                return (
                  <ol key={pIndex} className="list-decimal list-inside space-y-1 my-1.5 pl-1 text-xs sm:text-sm">
                    {items.map((item, i) => (
                      <li key={i}>{renderInline(item)}</li>
                    ))}
                  </ol>
                );
              }

              // Heading
              if (trimmed.startsWith("### ")) {
                return (
                  <h4 key={pIndex} className="font-semibold text-foreground text-sm pt-1">
                    {renderInline(trimmed.replace(/^###\s+/, ""))}
                  </h4>
                );
              }
              if (trimmed.startsWith("## ")) {
                return (
                  <h3 key={pIndex} className="font-semibold text-foreground text-base pt-1">
                    {renderInline(trimmed.replace(/^##\s+/, ""))}
                  </h3>
                );
              }

              // Standard paragraph with inline line breaks
              const lines = para.split("\n");
              return (
                <p key={pIndex} className="my-1">
                  {lines.map((line, lIdx) => (
                    <React.Fragment key={lIdx}>
                      {lIdx > 0 && <br />}
                      {renderInline(line)}
                    </React.Fragment>
                  ))}
                </p>
              );
            })}
          </React.Fragment>
        );
      })}
    </div>
  );
}

function renderInline(text: string): React.ReactNode[] {
  // Parse inline code, bold, links
  const tokens = text.split(/(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*)/g);

  return tokens.map((token, idx) => {
    if (token.startsWith("`") && token.endsWith("`") && token.length > 2) {
      return (
        <code
          key={idx}
          className="rounded bg-muted px-1.5 py-0.5 font-mono text-[12px] text-foreground border border-border/50"
        >
          {token.slice(1, -1)}
        </code>
      );
    }
    if (token.startsWith("**") && token.endsWith("**") && token.length > 4) {
      return (
        <strong key={idx} className="font-semibold text-foreground">
          {token.slice(2, -2)}
        </strong>
      );
    }
    if (token.startsWith("*") && token.endsWith("*") && token.length > 2) {
      return (
        <em key={idx} className="italic text-foreground/90">
          {token.slice(1, -1)}
        </em>
      );
    }
    return token;
  });
}
