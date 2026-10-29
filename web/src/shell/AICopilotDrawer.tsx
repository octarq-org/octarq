import React, { useCallback, useEffect, useRef } from "react";
import { create } from "zustand";
import { AlertCircle, Mail, Search, Send, Sparkles, Square, Sunrise, TrendingUp, Trash2, X } from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { useTranslation, type TFunc } from "../i18n";
import { ThinkingCollapsible } from "./command/ThinkingCollapsible";
import { streamChatResponse, type ChatFailure } from "./command/streamChat";
import type { ChatMessage } from "./command/types";
import { A2UIRenderer, type A2UIWidget } from "../components/a2ui";
import { CopilotAgentPicker, useWorkforceAgents } from "./CopilotAgentPicker";
import { roleSatisfies, useCurrentRole } from "./role";

export interface AICopilotUIState {
  isOpen: boolean;
  messages: ChatMessage[];
  input: string;
  isStreaming: boolean;
  error: string | null;
  // agentId charges the next turns to a workforce agent's budget. An empty
  // value leaves the turns unbilled.
  agentId: string;

  openDrawer: () => void;
  closeDrawer: () => void;
  toggleDrawer: () => void;
  setInput: (input: string) => void;
  setMessages: (messages: ChatMessage[] | ((prev: ChatMessage[]) => ChatMessage[])) => void;
  addMessage: (msg: ChatMessage) => void;
  updateLastMessage: (updater: Partial<ChatMessage> | ((prev: ChatMessage) => ChatMessage)) => void;
  setIsStreaming: (isStreaming: boolean) => void;
  setError: (error: string | null) => void;
  setAgentId: (agentId: string) => void;
  clearMessages: () => void;
  resetDrawer: () => void;
}

export const useAICopilotStore = create<AICopilotUIState>((set) => ({
  isOpen: false,
  messages: [],
  input: "",
  isStreaming: false,
  error: null,
  agentId: "",

  openDrawer: () => set({ isOpen: true }),
  closeDrawer: () => set({ isOpen: false }),
  toggleDrawer: () => set((state) => ({ isOpen: !state.isOpen })),
  setInput: (input) => set({ input }),
  setAgentId: (agentId) => set({ agentId }),

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
  resetDrawer: () =>
    set({ isOpen: false, messages: [], input: "", isStreaming: false, error: null, agentId: "" }),
}));

// agentFailureText turns a billing refusal into the next action an operator
// can take. The server message is the last resort, not the default.
function agentFailureText(t: TFunc, failure: ChatFailure | undefined, fallback: string): string {
  switch (failure?.code) {
    case "workforce_agent_paused":
      return t("copilot.agentPaused", "This agent is paused. Resume it in AI Workforce.");
    case "workforce_budget_exceeded":
      return t(
        "copilot.agentBudgetExceeded",
        "This agent reached its monthly budget limit. Raise the limit in AI Workforce.",
      );
    case "workforce_model_price_missing":
      return t("copilot.agentPriceMissing", "No price is set for this model. Add the price in AI Workforce.");
    case "workforce_spend_not_recorded":
      return t("copilot.agentSpendNotRecorded", "The turn finished. Its spend was not saved. Check the billing setup.");
    case "workforce_agent_not_found":
      return t("copilot.agentNotFound", "This agent no longer exists. Select another agent.");
  }
  if (failure?.status === 403 && failure.code === "forbidden") {
    return t("copilot.agentForbidden", "Only an admin can bill a turn to an agent.");
  }
  return fallback;
}

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
        "group tap-expand relative flex h-9 items-center gap-1.5 rounded-xl border px-2.5 text-xs font-semibold transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-border",
        isOpen
          ? "border-primary/40 bg-accent-soft text-accent-fg ring-1 ring-accent-border/30"
          : "border-border bg-surface-hover/50 hover:bg-surface-hover hover:border-border-strong text-muted-foreground hover:text-foreground",
        className,
      )}
      data-testid="ai-copilot-trigger-btn"
    >
      <div className="relative flex items-center justify-center">
        <Sparkles className="h-4 w-4 text-accent-fg transition-transform group-hover:scale-110" aria-hidden="true" />
      </div>
      <span className="hidden sm:inline font-medium group-hover:text-foreground">
        {t("copilot.drawerTitle", "AI Copilot")}
      </span>
      <kbd className="hidden rounded-md border border-border bg-muted px-1 py-0.5 text-[10px] font-mono font-medium text-muted-foreground md:block">
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
    agentId,
    setAgentId,
  } = useAICopilotStore();

  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  // Attribution spends workspace money, so the control only exists for an
  // admin. The server enforces the same rule.
  const { role, isInstanceAdmin } = useCurrentRole();
  const canAttribute = roleSatisfies("admin", role, isInstanceAdmin);
  const { agents, failed: agentsFailed, loaded: agentsLoaded, refresh: refreshAgents } =
    useWorkforceAgents(isOpen && canAttribute);

  // Drop a selection whose agent was deleted since the list was read.
  useEffect(() => {
    if (!agentsLoaded || !agentId) return;
    if (!agents.some((a) => a.id === agentId)) setAgentId("");
  }, [agents, agentsLoaded, agentId, setAgentId]);

  // Drop the selection when the caller is no longer allowed to bill a turn.
  useEffect(() => {
    if (role !== undefined && !canAttribute && agentId) setAgentId("");
  }, [role, canAttribute, agentId, setAgentId]);

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
        agentId: canAttribute && agentId ? agentId : undefined,
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
        onError: (err, failure) => {
          setError(canAttribute && agentId ? agentFailureText(t, failure, err) : err);
          setIsStreaming(false);
        },
        onDone: () => {
          setIsStreaming(false);
        },
      });

      // The response body ends after the server records the spend, so the
      // refreshed figures include this turn.
      if (canAttribute && agentId) void refreshAgents();
    },
    [
      input,
      isStreaming,
      messages,
      setError,
      setIsStreaming,
      setInput,
      setMessages,
      updateLastMessage,
      canAttribute,
      agentId,
      refreshAgents,
      t,
    ],
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
              <h2 className="flex items-center gap-2 font-display text-[15px] font-semibold tracking-[-0.01em] text-foreground">
                {t("copilot.drawerTitle", "AI Copilot")}
                <span className="rounded-full bg-accent-soft px-2 py-0.5 text-[10px] font-medium text-accent-fg">
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
          className="min-h-0 flex-1 overflow-y-auto p-4 space-y-4 scrollbar-thin"
        >
          {messages.length === 0 ? (
            <div data-testid="copilot-empty-state" className="flex flex-col items-center justify-center gap-5 py-8 text-center">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl bg-accent-soft text-accent-fg shadow-[var(--shadow-card)] ring-1 ring-accent-border">
                <Sparkles className="h-5 w-5" />
              </div>
              <div className="max-w-xs space-y-1.5">
                <h3 className="font-display text-[15px] font-semibold tracking-[-0.01em] text-foreground">
                  {t("copilot.drawerTitle", "AI Copilot")}
                </h3>
                <p className="text-xs leading-relaxed text-muted-foreground">
                  {t("copilot.placeholder", "Ask anything about your workspace...")}
                </p>
              </div>

              {/* Quick Prompt Pills */}
              <div className="w-full space-y-2 pt-1 text-left">
                {[
                  {
                    id: "p0",
                    icon: <Sunrise className="h-3.5 w-3.5" />,
                    text: t("copilot.quickPrompt0", "生成 6 维晨间工作简报 (Briefing v2) 与多维异动播报"),
                  },
                  {
                    id: "p1",
                    icon: <TrendingUp className="h-3.5 w-3.5" />,
                    text: t("copilot.quickPrompt1", "分析过去 7 天短链点击趋势与异常来源"),
                  },
                  {
                    id: "p2",
                    icon: <Search className="h-3.5 w-3.5" />,
                    text: t("copilot.quickPrompt2", "检查当前 DNS 记录与域名健康状态"),
                  },
                  {
                    id: "p3",
                    icon: <Mail className="h-3.5 w-3.5" />,
                    text: t("copilot.quickPrompt3", "汇总最新未读邮件并生成摘要"),
                  },
                ].map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => handleSend(item.text)}
                    className="group flex w-full cursor-pointer items-start gap-2.5 rounded-xl border border-border bg-card p-2.5 text-left text-xs text-foreground transition-[background-color,border-color,transform] duration-[var(--dur-fast)] ease-[var(--ease-out)] hover:-translate-y-px hover:border-border-strong hover:bg-surface-hover"
                  >
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-fg">
                      {item.icon}
                    </span>
                    <span className="flex-1 font-medium leading-relaxed transition-colors group-hover:text-foreground">
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
                <div className="px-1 font-mono text-[10px] uppercase tracking-[0.08em] text-muted-foreground">
                  {msg.role === "user" ? t("copilot.userLabel", "You") : t("copilot.assistantName", "Octarq Copilot")}
                </div>
                <div
                  className={cn(
                    "max-w-[88%] rounded-2xl p-3 text-xs leading-relaxed transition-colors",
                    msg.role === "user"
                      ? "rounded-2xl rounded-tr-md bg-primary-solid px-3 py-2 text-xs font-medium leading-relaxed text-primary-foreground"
                      : "rounded-2xl rounded-tl-md border border-border bg-card px-3 py-2 text-xs leading-relaxed text-foreground",
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
                    <div className="mt-2 space-y-1.5 pt-2 border-t border-border">
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
              className="flex items-start gap-2 rounded-xl border border-danger-border bg-danger-bg p-3 text-xs text-danger-fg"
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
          {canAttribute && (agents.length > 0 || agentsFailed) && (
            <CopilotAgentPicker
              agents={agents}
              value={agentId}
              onChange={setAgentId}
              loadFailed={agentsFailed}
            />
          )}
          <div className="relative flex items-center rounded-xl border border-input bg-card p-1 shadow-[inset_0_1px_1px_rgb(0_0_0/0.02)] transition-[border-color,box-shadow] duration-[var(--dur-fast)] ease-[var(--ease-out)] focus-within:border-accent-border focus-within:ring-2 focus-within:ring-accent-soft">
            <textarea
              ref={inputRef}
              rows={2}
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              aria-label={t("copilot.placeholder", "Ask anything about your workspace...")}
              placeholder={t("copilot.placeholder", "Ask anything about your workspace...")}
              className="w-full resize-none bg-transparent px-2.5 py-1.5 text-[13px] leading-5 text-foreground placeholder:text-muted-foreground focus:outline-none"
            />
            <div className="flex items-center gap-1 self-end pr-1 pb-1">
              {isStreaming ? (
                <button
                  type="button"
                  onClick={handleStop}
                  className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg bg-danger-solid text-danger-solid-fg transition-colors hover:brightness-110"
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
                  className="flex h-8 w-8 cursor-pointer items-center justify-center rounded-lg bg-primary-solid text-primary-foreground shadow-[var(--shadow-accent)] transition-colors hover:bg-primary-solid-hover disabled:pointer-events-none disabled:opacity-40"
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
