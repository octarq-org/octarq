import { ArrowLeft, ArrowUp, Sparkles, Square } from "lucide-react";
import { useTranslation } from "../../i18n";
import { A2UIRenderer, type A2UIWidget } from "../../components/a2ui";
import { ThinkingCollapsible } from "./ThinkingCollapsible";
import type { ChatMessage } from "./types";

export interface ChatViewProps {
  messages: ChatMessage[];
  chatInput: string;
  isStreaming: boolean;
  chatError: string | null;
  chatScrollRef: React.RefObject<HTMLDivElement>;
  chatInputRef: React.RefObject<HTMLTextAreaElement>;
  onChatInputChange: (v: string) => void;
  onSend: () => void;
  onStop: () => void;
  onBackToSearch: () => void;
}

export function ChatView({
  messages,
  chatInput,
  isStreaming,
  chatError,
  chatScrollRef,
  chatInputRef,
  onChatInputChange,
  onSend,
  onStop,
  onBackToSearch,
}: ChatViewProps) {
  const { t } = useTranslation();

  return (
    <>
      {/* Chat Header */}
      <div className="flex items-center justify-between border-b border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.015] dark:bg-white/[0.015] px-4 py-2.5 transition-colors">
        <button
          type="button"
          onClick={onBackToSearch}
          className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
          aria-label={t("command.chat.backToSearch")}
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          <span>{t("command.chat.backToSearch")}</span>
        </button>
        <div className="flex items-center gap-1.5 text-xs font-semibold text-primary">
          <Sparkles className="h-3.5 w-3.5" />
          <span>{t("command.chat.modeBadge")}</span>
        </div>
        <kbd className="shrink-0 rounded-md border border-foreground/10 dark:border-white/10 bg-muted/60 px-1.5 py-0.5 text-[10px] font-mono font-medium text-muted-foreground">
          esc
        </kbd>
      </div>

      {/* Chat Message List */}
      <div
        ref={chatScrollRef}
        className="max-h-[48vh] min-h-[180px] overflow-y-auto p-4 space-y-3.5 scrollbar-thin"
      >
        {messages.length === 0 ? (
          <div className="px-3 py-10 text-center">
            <div className="mx-auto w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center mb-3">
              <Sparkles className="h-5 w-5 text-primary" />
            </div>
            <p className="text-sm font-semibold text-foreground">{t("command.chat.askAi")}</p>
            <p className="mx-auto mt-1 max-w-sm text-xs leading-relaxed text-muted-foreground">
              {t("command.chat.placeholder")}
            </p>
          </div>
        ) : (
          messages.map((msg, i) => (
            <div
              key={msg.id}
              className={`flex flex-col ${msg.role === "user" ? "items-end" : "items-start w-full"}`}
            >
              <div className="text-[10px] font-medium text-muted-foreground/70 mb-1 px-1">
                {msg.role === "user" ? t("command.chat.user") : t("command.chat.assistant")}
              </div>
              {msg.role === "user" ? (
                <div className="max-w-[85%] rounded-2xl rounded-tr-sm bg-primary px-3.5 py-2 text-sm text-primary-foreground">
                  <p className="whitespace-pre-wrap leading-relaxed">{msg.content}</p>
                </div>
              ) : (
                <div className="w-full rounded-2xl rounded-tl-sm bg-foreground/[0.025] dark:bg-white/[0.035] border border-foreground/[0.06] dark:border-white/[0.06] p-3 text-sm text-foreground">
                  {msg.thinking && (
                    <ThinkingCollapsible thinking={msg.thinking} tokens={msg.tokens} />
                  )}
                  {msg.content && (
                    <p className="whitespace-pre-wrap leading-relaxed">{msg.content}</p>
                  )}
                  {msg.toolCalls?.map((tc, tcIdx) => {
                    const widget =
                      typeof tc === "object" && tc !== null && "a2ui" in tc && tc.a2ui
                        ? (tc.a2ui as A2UIWidget)
                        : (tc as A2UIWidget);
                    return <A2UIRenderer key={tcIdx} widget={widget} />;
                  })}
                  {isStreaming &&
                    i === messages.length - 1 &&
                    !msg.content &&
                    !msg.thinking &&
                    (!msg.toolCalls || msg.toolCalls.length === 0) && (
                      <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground animate-pulse py-1">
                        <Sparkles className="h-3.5 w-3.5 text-primary" />
                        <span>{t("command.chat.thinking")}…</span>
                      </span>
                    )}
                </div>
              )}
            </div>
          ))
        )}

        {chatError && (
          <div
            role="alert"
            className="rounded-lg border border-destructive/20 bg-destructive/10 p-2.5 text-xs text-destructive flex items-center justify-between"
          >
            <span>
              {t("command.chat.error")}: {chatError}
            </span>
          </div>
        )}
      </div>

      {/* Chat Input Box */}
      <div className="border-t border-foreground/[0.08] dark:border-white/[0.08] p-3 bg-foreground/[0.015] dark:bg-white/[0.015]">
        <div className="flex items-end gap-2">
          <textarea
            ref={chatInputRef}
            rows={1}
            value={chatInput}
            onChange={(e) => onChatInputChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                onSend();
              } else if (e.key === "Escape") {
                e.preventDefault();
                onBackToSearch();
              }
            }}
            placeholder={t("command.chat.placeholder")}
            aria-label={t("command.chat.inputLabel")}
            className="flex-1 resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground/60 outline-none border-none ring-0 focus:outline-none focus:border-none focus:ring-0 max-h-28 py-1.5 scrollbar-thin"
          />
          {isStreaming ? (
            <button
              type="button"
              onClick={onStop}
              className="shrink-0 flex items-center gap-1 rounded-lg bg-muted/80 hover:bg-muted px-2.5 py-1.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
            >
              <Square className="h-3.5 w-3.5" />
              <span>{t("command.chat.stop")}</span>
            </button>
          ) : (
            <button
              type="button"
              onClick={onSend}
              disabled={!chatInput.trim()}
              aria-label={t("command.chat.send")}
              className="shrink-0 flex items-center justify-center rounded-lg bg-primary p-2 text-primary-foreground transition-opacity disabled:opacity-40 cursor-pointer disabled:cursor-not-allowed"
            >
              <ArrowUp className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>
    </>
  );
}
