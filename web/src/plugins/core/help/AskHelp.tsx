import { useEffect, useState, type FormEvent } from "react";
import { Send, Sparkles } from "lucide-react";
import { api } from "../../../api";
import { useTranslation, type Lang } from "../../../i18n";
import { useAICopilotStore } from "../../../shell/AICopilotDrawer";

export const ASK_HELP_EXCERPT_LIMIT = 6000;
export const ASK_HELP_QUESTION_LIMIT = 500;

export interface HelpPromptInput {
  slug: string;
  lang: Lang;
  html: string;
  question: string;
}

export function helpExcerpt(html: string, limit = ASK_HELP_EXCERPT_LIMIT): string {
  const spaced = html
    .replace(/<br\s*\/?>/gi, " ")
    .replace(
      /<\/(?:p|div|section|article|li|ul|ol|tr|td|th|h[1-6]|pre|blockquote|table|thead|tbody)>/gi,
      "$& ",
    );
  const parsed = new DOMParser().parseFromString(spaced, "text/html");
  const text = (parsed.body.textContent ?? "").replace(/\s+/g, " ").trim();
  if (text.length <= limit) return text;
  return `${text.slice(0, limit)}\n[truncated]`;
}

export function buildHelpPrompt({ slug, lang, html, question }: HelpPromptInput): string {
  const asked = question.trim().slice(0, ASK_HELP_QUESTION_LIMIT);
  return [
    "/help",
    `Article: ${slug}`,
    `Locale: ${lang}`,
    "",
    helpExcerpt(html),
    "",
    `Question: ${asked}`,
  ].join("\n");
}

type AssistState = "checking" | "ready" | "hidden";

export function AskHelp({ slug, lang, html }: { slug: string; lang: Lang; html: string }) {
  const { t } = useTranslation();
  const [state, setState] = useState<AssistState>("checking");
  const [question, setQuestion] = useState("");
  const addMessage = useAICopilotStore((s) => s.addMessage);
  const openDrawer = useAICopilotStore((s) => s.openDrawer);

  useEffect(() => {
    let active = true;
    setState("checking");
    api
      .aiAssistStatus()
      .then((status) => {
        if (active) setState(status.configured ? "ready" : "hidden");
      })
      .catch(() => {
        if (active) setState("hidden");
      });
    return () => {
      active = false;
    };
  }, [slug]);

  if (state !== "ready") return null;

  const send = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const asked = question.trim();
    if (!asked) return;
    addMessage({
      id: `help-ask-${slug}-${Date.now()}`,
      role: "user",
      content: buildHelpPrompt({ slug, lang, html, question: asked }),
    });
    setQuestion("");
    openDrawer();
  };

  return (
    <div
      data-testid="help-ask"
      className="mt-4 flex flex-col gap-2 rounded-lg border border-border bg-card p-3 sm:flex-row sm:items-center"
    >
      <span className="flex shrink-0 items-center gap-1.5 text-xs font-semibold text-foreground">
        <Sparkles className="h-3.5 w-3.5 text-accent-fg" />
        {t("help.ask_ai", "Ask AI")}
      </span>
      <form onSubmit={send} className="flex min-w-0 flex-1 items-center gap-2">
        <input
          type="text"
          value={question}
          onChange={(event) => setQuestion(event.target.value)}
          maxLength={ASK_HELP_QUESTION_LIMIT}
          aria-label={t("help.ask_ai_label", "Question about this page")}
          placeholder={t("help.ask_ai_placeholder", "Ask about this page")}
          className="min-w-0 flex-1 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-border"
        />
        <button
          type="submit"
          disabled={!question.trim()}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-accent-border bg-accent-soft px-3 py-1.5 text-xs font-semibold text-accent-fg transition-colors hover:bg-surface-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          <Send className="h-3.5 w-3.5" />
          {t("help.ask_ai_send", "Ask")}
        </button>
      </form>
    </div>
  );
}
