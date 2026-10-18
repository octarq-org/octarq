import React, { useCallback, useEffect, useRef } from "react";
import { create } from "zustand";
import { AlertCircle, Send, Sparkles, Square, Trash2, X } from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { useTranslation } from "../i18n";
import { ThinkingCollapsible } from "./command/ThinkingCollapsible";
import { streamChatResponse } from "./command/streamChat";
import type { ChatMessage } from "./command/types";
import { A2UIRenderer, type A2UIWidget } from "../components/a2ui";

export interface AICopilotUIState {
  isOpen: boolean;
  messages: ChatMessage[];
  input: string;
  isStreaming: boolean;
  error: string | null;

  openDrawer: () => void;
  closeDrawer: () => void;
  toggleDrawer: () => void;
  setInput: (input: string) => void;
  setMessages: (messages: ChatMessage[] | ((prev: ChatMessage[]) => ChatMessage[])) => void;
  addMessage: (msg: ChatMessage) => void;
  updateLastMessage: (updater: Partial<ChatMessage> | ((prev: ChatMessage) => ChatMessage)) => void;
  setIsStreaming: (isStreaming: boolean) => void;
  setError: (error: string | null) => void;
  clearMessages: () => void;
  resetDrawer: () => void;
}

export const useAICopilotStore = create<AICopilotUIState>((set) => ({
  isOpen: false,
  messages: [],
  input: "",
  isStreaming: false,
  error: null,

  openDrawer: () => set({ isOpen: true }),
  closeDrawer: () => set({ isOpen: false }),
  toggleDrawer: () => set((state) => ({ isOpen: !state.isOpen })),
  setInput: (input) => set({ input }),

  setMessages: (messages) =>
    set((state) => ({
      messages: typeof messages === "function" ? messages(state.messages) : messages,
    })),

  addMessage: (msg) =>
    set((state) => ({
      messages: [...state.messages, msg],
    })),

  updateLastMessage: (updater) =>
    set((state) => {
      if (state.messages.length === 0) return state;
      const lastIdx = state.messages.length - 1;
      const last = state.messages[lastIdx];
      const updated = typeof updater === "function" ? updater(last) : { ...last, ...updater };
      const next = [...state.messages];
      next[lastIdx] = updated;
      return { messages: next };
    }),

  setIsStreaming: (isStreaming) => set({ isStreaming }),
  setError: (error) => set({ error }),
  clearMessages: () => set({ messages: [], input: "", error: null }),
  resetDrawer: () => set({ isOpen: false, messages: [], input: "", isStreaming: false, error: null }),
}));

/**
 * Sparkles trigger button in global TopBar header.
 */
export function AICopilotButton({ className }: { className?: string }) {
  const { t } = useTranslation();
  const isOpen = useAICopilotStore((s) => s.isOpen);
  const toggleDrawer = useAICopilotStore((s) => s.toggleDrawer);

  return (
    <button
      type="button"
      onClick={toggleDrawer}
      aria-label={t("copilot.drawerTitle", "AI Copilot")}
      title={`${t("copilot.drawerTitle", "AI Copilot")} (⌘J)`}
      aria-pressed={isOpen}
      className={cn(
        "group relative flex h-9 items-center gap-1.5 rounded-xl border px-2.5 text-xs font-semibold transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary",
        isOpen
          ? "border-primary/40 bg-primary/15 text-primary ring-1 ring-primary/30"
          : "border-foreground/10 dark:border-white/10 bg-surface-hover/50 hover:bg-surface-hover hover:border-foreground/20 text-muted-foreground hover:text-foreground",
        className,
      )}
      data-testid="ai-copilot-trigger-btn"
    >
      <div className="relative flex items-center justify-center">
        <Sparkles className="h-4 w-4 text-primary transition-transform group-hover:scale-110" aria-hidden="true" />
      </div>
      <span className="hidden sm:inline font-medium group-hover:text-foreground">
        {t("copilot.drawerTitle", "AI Copilot")}
      </span>
      <kbd className="hidden rounded-md border border-foreground/10 dark:border-white/10 bg-muted/80 px-1 py-0.5 text-[9px] font-mono font-medium text-muted-foreground md:block">
        {t("copilot.shortcutKey", "⌘J")}
      </kbd>
    </button>
  );
}

/**
 * Inline AI Copilot Drawer component with streaming, thinking collapsible card,
 * and quick prompt cards.
 */
export function AICopilotDrawer() {
  const { t } = useTranslation();
  const {
    isOpen,
    closeDrawer,
    toggleDrawer,
    openDrawer,
    messages,
    setMessages,
    updateLastMessage,
    input,
    setInput,
    isStreaming,
    setIsStreaming,
    error,
    setError,
    clearMessages,
  } = useAICopilotStore();

  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Global keydown listeners for Mod+J and Mod+/ shortcuts
  useEffect(() => {
    const handleGlobalKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && (e.key === "j" || e.key === "J" || e.key === "/")) {
        e.preventDefault();
        toggleDrawer();
      }
    };

    const handleOpenEvent = () => {
      openDrawer();
    };

    window.addEventListener("keydown", handleGlobalKeyDown);
    window.addEventListener("octarq:open-copilot", handleOpenEvent);
    return () => {
      window.removeEventListener("keydown", handleGlobalKeyDown);
      window.removeEventListener("octarq:open-copilot", handleOpenEvent);
    };
  }, [toggleDrawer, openDrawer]);

  // Focus input and cancel active stream on drawer close
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 100);
    } else {
      abortControllerRef.current?.abort();
      setIsStreaming(false);
    }
  }, [isOpen, setIsStreaming]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
    };
  }, []);

  // Auto-scroll chat to bottom
  useEffect(() => {
    if (isOpen) {
      scrollRef.current?.scrollTo({
        top: scrollRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [messages, isStreaming, isOpen]);

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort();
    setIsStreaming(false);
  }, [setIsStreaming]);

  const handleSend = useCallback(
    async (promptOverride?: string) => {
      const prompt = (promptOverride !== undefined ? promptOverride : input).trim();
      if (!prompt || isStreaming) return;

      setInput("");
      setError(null);

      const userMsg: ChatMessage = {
        id: `user-${Date.now()}`,
        role: "user",
        content: prompt,
      };

      const asstMsgId = `asst-${Date.now()}`;
      const asstMsg: ChatMessage = {
        id: asstMsgId,
        role: "assistant",
        content: "",
        thinking: "",
        toolCalls: [],
      };

      const newHistory = [...messages, userMsg];
      setMessages([...newHistory, asstMsg]);

      abortControllerRef.current?.abort();
      const controller = new AbortController();
      abortControllerRef.current = controller;
      setIsStreaming(true);

      const outboundMessages = newHistory.map((m) => ({
        role: m.role,
        content: m.content,
      }));

      await streamChatResponse({
        messages: outboundMessages,
        signal: controller.signal,
        onThinking: (delta, tokens) => {
          updateLastMessage((prev) => ({
            ...prev,
            thinking: (prev.thinking || "") + delta,
            tokens: tokens !== undefined ? tokens : prev.tokens,
          }));
        },
        onText: (delta) => {
          updateLastMessage((prev) => ({
            ...prev,
            content: prev.content + delta,
          }));
        },
        onTool: (toolPayload) => {
          updateLastMessage((prev) => ({
            ...prev,
            toolCalls: [...(prev.toolCalls || []), toolPayload],
          }));
        },
        onError: (err) => {
          setError(err);
          setIsStreaming(false);
        },
        onDone: () => {
          setIsStreaming(false);
        },
      });
    },
    [input, isStreaming, messages, setError, setIsStreaming, setInput, setMessages, updateLastMessage],
  );

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Escape") {
      e.preventDefault();
      closeDrawer();
      return;
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 backdrop-blur-xs transition-opacity animate-in fade-in"
        onClick={closeDrawer}
        aria-hidden="true"
        data-testid="copilot-backdrop"
      />

      {/* Drawer Panel */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t("copilot.drawerTitle", "AI Copilot")}
        className="relative z-50 flex h-full w-full max-w-md flex-col border-l border-border bg-background shadow-2xl transition-all duration-200 animate-in slide-in-from-right"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-4 py-3.5">
          <div className="flex items-center gap-2">
            <div className="flex h-7 w-7 items-center justify-center rounded-lg brand-gradient text-white shadow-xs">
              <Sparkles className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-foreground flex items-center gap-2">
                {t("copilot.drawerTitle", "AI Copilot")}
                <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[10px] font-medium text-primary">
                  {isStreaming ? t("copilot.thinkingProcess", "Streaming") : t("copilot.ready", "Ready")}
                </span>
              </h2>
            </div>
          </div>

          <div className="flex items-center gap-1">
            {messages.length > 0 && (
              <button
                type="button"
                onClick={clearMessages}
                aria-label={t("copilot.clearChat", "Clear chat")}
                title={t("copilot.clearChat", "Clear chat")}
                className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors cursor-pointer"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button
              type="button"
              onClick={closeDrawer}
              aria-label={t("common.close", "Close")}
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors cursor-pointer"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Messages Body */}
        <div
          ref={scrollRef}
          className="flex-1 overflow-y-auto p-4 space-y-4 scrollbar-thin"
        >
          {messages.length === 0 ? (
            <div data-testid="copilot-empty-state" className="flex flex-col items-center justify-center py-8 text-center space-y-4">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary border border-primary/20 shadow-xs">
                <Sparkles className="h-6 w-6" />
              </div>
              <div className="max-w-xs space-y-1">
                <h3 className="text-sm font-semibold text-foreground">
                  {t("copilot.drawerTitle", "AI Copilot")}
                </h3>
                <p className="text-xs text-muted-foreground">
                  {t("copilot.placeholder", "Ask anything about your workspace...")}
                </p>
              </div>

              {/* Quick Prompt Pills */}
              <div className="w-full space-y-2 pt-2 text-left">
                {[
                  {
                    id: "p1",
                    icon: "💡",
                    text: t("copilot.quickPrompt1", "分析过去 7 天短链点击趋势与异常来源"),
                  },
                  {
                    id: "p2",
                    icon: "🔍",
                    text: t("copilot.quickPrompt2", "检查当前 DNS 记录与域名健康状态"),
                  },
                  {
                    id: "p3",
                    icon: "✉️",
                    text: t("copilot.quickPrompt3", "汇总最新未读邮件并生成摘要"),
                  },
                ].map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => handleSend(item.text)}
                    className="flex w-full items-start gap-2.5 rounded-xl border border-border bg-surface-hover/40 hover:bg-surface-hover hover:border-primary/30 p-2.5 text-xs text-foreground/90 transition-all text-left cursor-pointer group"
                  >
                    <span className="text-sm shrink-0">{item.icon}</span>
                    <span className="flex-1 font-medium leading-relaxed group-hover:text-primary transition-colors">
                      {item.text}
                    </span>
                  </button>
                ))}
              </div>
            </div>
          ) : (
            messages.map((msg) => (
              <div
                key={msg.id}
                className={cn(
                  "flex flex-col space-y-1",
                  msg.role === "user" ? "items-end" : "items-start",
                )}
              >
                <div className="text-[10px] font-mono text-muted-foreground/70 px-1">
                  {msg.role === "user" ? t("copilot.userLabel", "You") : t("copilot.assistantName", "Octarq Copilot")}
                </div>
                <div
                  className={cn(
                    "max-w-[88%] rounded-2xl p-3 text-xs leading-relaxed transition-colors",
                    msg.role === "user"
                      ? "bg-primary text-primary-foreground font-medium rounded-tr-xs"
                      : "bg-surface-hover border border-border text-foreground rounded-tl-xs",
                  )}
                >
                  {msg.role === "assistant" && msg.thinking && (
                    <ThinkingCollapsible
                      thinking={msg.thinking}
                      tokens={msg.tokens}
                      defaultOpen={isStreaming && !msg.content}
                    />
                  )}
                  {msg.content ? (
                    <div className="whitespace-pre-wrap font-sans">{msg.content}</div>
                  ) : msg.role === "assistant" && isStreaming && !msg.thinking ? (
                    <div className="flex items-center gap-1.5 py-1 text-muted-foreground">
                      <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-primary" />
                      <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:0.2s]" />
                      <span className="h-1.5 w-1.5 animate-bounce rounded-full bg-primary [animation-delay:0.4s]" />
                    </div>
                  ) : null}
                  {msg.toolCalls && msg.toolCalls.length > 0 && (
                    <div className="mt-2 space-y-1.5 pt-2 border-t border-border/50">
                      {msg.toolCalls.map((tc, tcIdx) => {
                        const widget =
                          typeof tc === "object" && tc !== null && "a2ui" in tc && tc.a2ui
                            ? (tc.a2ui as A2UIWidget)
                            : (tc as A2UIWidget);
                        return <A2UIRenderer key={tcIdx} widget={widget} />;
                      })}
                    </div>
                  )}
                </div>
              </div>
            ))
          )}

          {/* Stream Error Alert */}
          {error && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-xl border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive"
            >
              <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
              <div className="flex-1">
                <p className="font-semibold">{t("copilot.errorLabel", "Request Error")}</p>
                <p className="mt-0.5 text-muted-foreground/90">{error}</p>
              </div>
            </div>
          )}
        </div>

        {/* Input Footer */}
        <div className="border-t border-border bg-background p-3 space-y-2">
          <div className="relative flex items-center rounded-xl border border-border bg-surface-hover/30 focus-within:border-primary/50 focus-within:ring-1 focus-within:ring-primary/20 transition-all p-1">
            <textarea
              ref={inputRef}
              rows={2}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              aria-label={t("copilot.placeholder", "Ask anything about your workspace...")}
              placeholder={t("copilot.placeholder", "Ask anything about your workspace...")}
              className="w-full resize-none bg-transparent px-2.5 py-1.5 text-xs text-foreground placeholder:text-muted-foreground/60 focus:outline-none"
            />
            <div className="flex items-center gap-1 self-end pr-1 pb-1">
              {isStreaming ? (
                <button
                  type="button"
                  onClick={handleStop}
                  className="flex h-7 w-7 items-center justify-center rounded-lg bg-destructive text-destructive-foreground hover:bg-destructive/90 transition-colors cursor-pointer"
                  title={t("copilot.stopGenerating", "Stop")}
                  aria-label={t("copilot.stopGenerating", "Stop")}
                >
                  <Square className="h-3.5 w-3.5 fill-current" />
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => handleSend()}
                  disabled={!input.trim()}
                  className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-40 disabled:pointer-events-none transition-colors cursor-pointer"
                  title={t("copilot.send", "Send")}
                  aria-label={t("copilot.send", "Send")}
                >
                  <Send className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          </div>
          <div className="flex items-center justify-between px-1 text-[10px] text-muted-foreground/70 font-mono">
            <span>{t("copilot.shortcutHint", "Shift + Enter for new line · Enter to send · ⌘J to toggle")}</span>
          </div>
        </div>
      </div>
    </div>
  );
}
