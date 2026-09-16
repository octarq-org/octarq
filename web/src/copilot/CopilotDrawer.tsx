import React, { useEffect, useRef, useState } from "react";
import {
  Sparkles,
  X,
  Trash2,
  Send,
  Square,
  ChevronDown,
  ChevronRight,
  ShieldAlert,
  MessageSquare,
  Bot,
  Flame,
  CreditCard,
  Globe2,
  FileCheck,
} from "lucide-react";
import { useTranslation } from "../i18n";
import { Button, cn } from "../ui";
import { useCopilotStore } from "./store";
import { useAIStatusQuery, streamAIChat } from "./api";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { ActionDiffCard } from "./ActionDiffCard";
import type { ChatMessage, AIStatus } from "./types";
import { QueryClientContext } from "@tanstack/react-query";

function CopilotDrawerContent({ aiStatus }: { aiStatus?: AIStatus }) {
  const { t } = useTranslation();
  const {
    isOpen,
    closeCopilot,
    activeTab,
    setActiveTab,
    messages,
    input,
    setInput,
    isStreaming,
    setIsStreaming,
    addMessage,
    updateLastMessage,
    clearMessages,
    pendingApprovals,
    addActionDiff,
  } = useCopilotStore();

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const [thinkingExpanded, setThinkingExpanded] = useState<Record<string, boolean>>({});
  const [approvalsFilter, setApprovalsFilter] = useState<"all" | "pending" | "resolved">("all");

  const pendingCount = pendingApprovals.filter((a) => a.status === "pending").length;

  // Auto-scroll to bottom when messages update
  useEffect(() => {
    if (activeTab === "chat") {
      messagesEndRef.current?.scrollIntoView?.({ behavior: "smooth" });
    }
  }, [messages, isStreaming, activeTab]);

  // Focus input when opened
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 150);
    }
  }, [isOpen]);

  // Keyboard shortcut ESC to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        closeCopilot();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, closeCopilot]);

  const handleSendMessage = async (textToSend?: string) => {
    const content = (textToSend ?? input).trim();
    if (!content || isStreaming) return;

    setInput("");

    const userMsg: ChatMessage = {
      id: `user-${Date.now()}`,
      role: "user",
      content,
      timestamp: Date.now(),
    };

    const assistantMsgId = `assistant-${Date.now()}`;
    const assistantMsg: ChatMessage = {
      id: assistantMsgId,
      role: "assistant",
      content: "",
      thinking: "",
      timestamp: Date.now(),
      isStreaming: true,
      actionDiffs: [],
    };

    addMessage(userMsg);
    addMessage(assistantMsg);
    setIsStreaming(true);

    const controller = new AbortController();
    abortControllerRef.current = controller;

    const chatHistory = [...messages, userMsg].map((m) => ({
      role: m.role,
      content: m.content,
    }));

    await streamAIChat({
      messages: chatHistory,
      signal: controller.signal,
      onThinking: (delta) => {
        updateLastMessage((prev) => ({
          ...prev,
          thinking: (prev.thinking || "") + delta,
        }));
      },
      onText: (delta) => {
        updateLastMessage((prev) => ({
          ...prev,
          content: (prev.content || "") + delta,
        }));
      },
      onToolCall: (toolCall) => {
        if (toolCall.args?.actionDiff) {
          const diff = toolCall.args.actionDiff as any;
          addActionDiff(diff);
          updateLastMessage((prev) => ({
            ...prev,
            actionDiffs: [...(prev.actionDiffs || []), diff],
          }));
        }
      },
      onError: (err) => {
        updateLastMessage((prev) => ({
          ...prev,
          content: prev.content
            ? prev.content + `\n\n> ❌ **${t("copilot.errorLabel", "请求异常")}**: ${err.message}`
            : `> ❌ **${t("copilot.errorLabel", "请求异常")}**: ${err.message}`,
          isStreaming: false,
        }));
      },
      onDone: () => {
        updateLastMessage((prev) => ({
          ...prev,
          isStreaming: false,
        }));
        setIsStreaming(false);
      },
    });

    setIsStreaming(false);
    abortControllerRef.current = null;
  };

  const handleStopGenerating = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
      setIsStreaming(false);
      updateLastMessage((prev) => ({
        ...prev,
        isStreaming: false,
      }));
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  const toggleThinking = (msgId: string) => {
    setThinkingExpanded((prev) => ({ ...prev, [msgId]: !prev[msgId] }));
  };

  if (!isOpen) return null;

  const filteredApprovals = pendingApprovals.filter((a) => {
    if (approvalsFilter === "pending") return a.status === "pending";
    if (approvalsFilter === "resolved") return a.status !== "pending";
    return true;
  });

  return (
    <div className="fixed inset-0 z-50 flex justify-end" data-testid="copilot-drawer">
      {/* Backdrop */}
      <div
        className="fixed inset-0 bg-black/40 backdrop-blur-xs transition-opacity animate-in fade-in"
        onClick={closeCopilot}
        aria-hidden="true"
      />

      {/* Drawer Container */}
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t("copilot.title", "AI Copilot")}
        className="relative z-50 flex h-full w-full max-w-lg flex-col border-l border-border bg-background shadow-2xl transition-all duration-200 animate-in slide-in-from-right"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-4 py-3 bg-well/40">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl brand-gradient text-white shadow-xs">
              <Sparkles className="h-4 w-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-bold text-foreground flex items-center gap-1.5">
                  {t("copilot.title", "AI Copilot")}
                </h2>
                {aiStatus?.configured ? (
                  <span className="inline-flex items-center gap-1 rounded-full bg-success-fg/10 px-2 py-0.5 text-[10px] font-semibold text-success-fg">
                    <span className="h-1.5 w-1.5 rounded-full bg-success-fg" />
                    <span>{aiStatus.provider || t("copilot.ready", "就绪")}</span>
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold text-amber-600 dark:text-amber-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
                    <span>{t("copilot.demoMode", "本地模式")}</span>
                  </span>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">
                {t("copilot.subtitle", "业务运营助理与人机协同中枢")}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-1">
            {messages.length > 0 && activeTab === "chat" && (
              <button
                type="button"
                onClick={clearMessages}
                aria-label={t("copilot.clearChat", "清空对话")}
                title={t("copilot.clearChat", "清空对话")}
                className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-danger-fg transition-colors"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            <button
              type="button"
              onClick={closeCopilot}
              aria-label={t("common.cancel", "关闭")}
              title={t("common.cancel", "关闭")}
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground transition-colors"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Tab Navigation */}
        <div className="flex border-b border-border bg-well/20 px-4 py-2">
          <div className="flex items-center gap-1 rounded-xl bg-surface-hover/50 p-0.5 w-full">
            <button
              type="button"
              onClick={() => setActiveTab("chat")}
              className={cn(
                "flex-1 flex items-center justify-center gap-1.5 rounded-lg py-1 text-xs font-medium transition-colors",
                activeTab === "chat"
                  ? "bg-background text-foreground shadow-xs font-semibold"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <MessageSquare className="h-3.5 w-3.5" />
              <span>{t("copilot.tabChat", "智能体对话")}</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("approvals")}
              className={cn(
                "flex-1 flex items-center justify-center gap-1.5 rounded-lg py-1 text-xs font-medium transition-colors",
                activeTab === "approvals"
                  ? "bg-background text-foreground shadow-xs font-semibold"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <ShieldAlert className="h-3.5 w-3.5" />
              <span>{t("copilot.tabApprovals", "待审卡片")}</span>
              {pendingCount > 0 && (
                <span className="rounded-full bg-danger-fg/15 px-1.5 py-0.2 text-[10px] font-bold text-danger-fg">
                  {pendingCount}
                </span>
              )}
            </button>
          </div>
        </div>

        {/* Tab Content: Chat */}
        {activeTab === "chat" && (
          <div className="flex flex-1 flex-col overflow-hidden">
            {/* Messages Area */}
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {messages.length === 0 ? (
                /* Welcome & Preset Prompts */
                <div className="my-auto py-6 space-y-6">
                  <div className="text-center space-y-2">
                    <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
                      <Bot className="h-6 w-6" />
                    </div>
                    <h3 className="text-sm font-bold text-foreground">
                      {t("copilot.welcomeTitle", "你好！我是 Octarq AI 运营协同助理")}
                    </h3>
                    <p className="text-xs text-muted-foreground max-w-sm mx-auto leading-relaxed">
                      {t(
                        "copilot.welcomeDesc",
                        "直接向我下达自然语言指令，支持短链汇总、支付核对、DNS 诊断与高危操作审批。",
                      )}
                    </p>
                  </div>

                  {/* Preset Action Suggestions */}
                  <div className="space-y-2">
                    <div className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground px-1">
                      {t("copilot.quickPrompts", "常用运营快捷指令")}
                    </div>
                    <div className="grid grid-cols-1 gap-2">
                      <PresetPromptCard
                        icon={<Flame className="h-4 w-4 text-orange-500" />}
                        title={t("copilot.promptLinks", "汇总本周点击最高的短链")}
                        desc={t("copilot.promptLinksDesc", "统计近 7 天访问趋势与热门重定向排行")}
                        onClick={() => handleSendMessage(t("copilot.promptLinks", "汇总本周点击最高的短链"))}
                      />
                      <PresetPromptCard
                        icon={<CreditCard className="h-4 w-4 text-emerald-500" />}
                        title={t("copilot.promptPayment", "查询最近一笔支付状态")}
                        desc={t("copilot.promptPaymentDesc", "核对 Stripe Webhook 履约与证书签发记录")}
                        onClick={() => handleSendMessage(t("copilot.promptPayment", "查询最近一笔支付状态"))}
                      />
                      <PresetPromptCard
                        icon={<Globe2 className="h-4 w-4 text-blue-500" />}
                        title={t("copilot.promptDns", "排查 api.octarq.com 的 DNS 异常")}
                        desc={t("copilot.promptDnsDesc", "检查权威解析节点延迟与 A/CNAME 配置一致性")}
                        onClick={() => handleSendMessage(t("copilot.promptDns", "排查 api.octarq.com 的 DNS 异常"))}
                      />
                      <PresetPromptCard
                        icon={<FileCheck className="h-4 w-4 text-purple-500" />}
                        title={t("copilot.promptDiff", "模拟生成高危操作审批卡片")}
                        desc={t("copilot.promptDiffDesc", "体验智能体发起破坏性变更的可视化审批流")}
                        onClick={() => handleSendMessage(t("copilot.promptDiff", "模拟生成高危操作审批卡片"))}
                      />
                    </div>
                  </div>
                </div>
              ) : (
                /* Chat Messages */
                messages.map((msg) => (
                  <div
                    key={msg.id}
                    className={cn(
                      "flex flex-col space-y-1.5",
                      msg.role === "user" ? "items-end" : "items-start",
                    )}
                  >
                    {/* Role / Name Header */}
                    <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground px-1">
                      {msg.role === "user" ? (
                        <span>{t("copilot.userLabel", "你")}</span>
                      ) : (
                        <span className="font-semibold text-primary flex items-center gap-1">
                          <Sparkles className="h-3 w-3" />
                          <span>{t("copilot.assistantName", "Octarq Copilot")}</span>
                        </span>
                      )}
                    </div>

                    {/* Thinking Process Accordion */}
                    {msg.thinking && (
                      <div className="w-full max-w-md rounded-xl border border-primary/20 bg-primary/5 p-2.5 text-xs text-muted-foreground my-1">
                        <button
                          type="button"
                          onClick={() => toggleThinking(msg.id)}
                          className="flex w-full items-center justify-between text-[11px] font-semibold text-primary hover:opacity-80"
                        >
                          <span className="flex items-center gap-1">
                            <Sparkles className="h-3 w-3" />
                            <span>{t("copilot.thinkingProcess", "思考过程")}</span>
                          </span>
                          {thinkingExpanded[msg.id] ? (
                            <ChevronDown className="h-3.5 w-3.5" />
                          ) : (
                            <ChevronRight className="h-3.5 w-3.5" />
                          )}
                        </button>
                        {thinkingExpanded[msg.id] && (
                          <div className="mt-2 pt-2 border-t border-primary/10 font-mono text-[11px] leading-relaxed break-words whitespace-pre-wrap">
                            {msg.thinking}
                          </div>
                        )}
                      </div>
                    )}

                    {/* Message Bubble */}
                    {msg.content && (
                      <div
                        className={cn(
                          "rounded-2xl px-3.5 py-2.5 max-w-[92%] shadow-xs",
                          msg.role === "user"
                            ? "bg-primary text-primary-foreground font-medium text-sm"
                            : "bg-surface-hover/80 text-foreground border border-border/80",
                        )}
                      >
                        {msg.role === "user" ? (
                          <div className="whitespace-pre-wrap break-words">{msg.content}</div>
                        ) : (
                          <MarkdownRenderer content={msg.content} />
                        )}

                        {/* Typing Cursor */}
                        {msg.isStreaming && (
                          <span className="inline-block h-3.5 w-1.5 ml-1 bg-primary animate-pulse" />
                        )}
                      </div>
                    )}

                    {/* Inline Action Diff Cards */}
                    {msg.actionDiffs && msg.actionDiffs.length > 0 && (
                      <div className="w-full space-y-2.5 pt-1 max-w-full">
                        {msg.actionDiffs.map((diff) => (
                          <ActionDiffCard key={diff.id} action={diff} />
                        ))}
                      </div>
                    )}
                  </div>
                ))
              )}
              <div ref={messagesEndRef} />
            </div>

            {/* Input & Footer Controls */}
            <div className="border-t border-border bg-well/30 p-3">
              <div className="relative flex flex-col rounded-2xl border border-border bg-background focus-within:border-primary/50 focus-within:ring-1 focus-within:ring-primary/50 transition-all">
                <textarea
                  ref={inputRef}
                  value={input}
                  onChange={(e) => setInput(e.target.value)}
                  onKeyDown={handleKeyDown}
                  placeholder={t("copilot.inputPlaceholder", "问点什么，或下达运营指令 (Enter 发送)...")}
                  rows={2}
                  className="w-full resize-none bg-transparent p-3 text-xs sm:text-sm text-foreground placeholder:text-muted-foreground outline-none"
                />

                <div className="flex items-center justify-between border-t border-border/40 px-3 py-2 bg-well/20 rounded-b-2xl">
                  <div className="text-[10px] text-muted-foreground hidden sm:block">
                    {t("copilot.shortcutHint", "Shift + Enter 换行 · Enter 发送 · ⌘J 唤起/收起")}
                  </div>

                  <div className="flex items-center gap-1.5 ml-auto">
                    {isStreaming ? (
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={handleStopGenerating}
                        className="h-7 px-2.5 text-xs flex items-center gap-1"
                        title={t("copilot.stopGenerating", "停止生成")}
                      >
                        <Square className="h-3 w-3 fill-current" />
                        <span>{t("copilot.stopGenerating", "停止生成")}</span>
                      </Button>
                    ) : (
                      <Button
                        variant="primary"
                        size="sm"
                        disabled={!input.trim()}
                        onClick={() => handleSendMessage()}
                        className="h-7 px-3 text-xs font-semibold flex items-center gap-1"
                        title={t("copilot.send", "发送")}
                      >
                        <span>{t("copilot.send", "发送")}</span>
                        <Send className="h-3 w-3" />
                      </Button>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Tab Content: Approvals */}
        {activeTab === "approvals" && (
          <div className="flex flex-1 flex-col overflow-hidden">
            {/* Filter Pills */}
            <div className="flex items-center justify-between border-b border-border bg-well/20 px-4 py-2.5">
              <div className="flex items-center gap-1 text-xs">
                <button
                  type="button"
                  onClick={() => setApprovalsFilter("all")}
                  className={cn(
                    "rounded-lg px-2.5 py-1 font-medium transition-colors",
                    approvalsFilter === "all"
                      ? "bg-surface-hover text-foreground font-semibold"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {t("copilot.filterAll", "全部")} ({pendingApprovals.length})
                </button>
                <button
                  type="button"
                  onClick={() => setApprovalsFilter("pending")}
                  className={cn(
                    "rounded-lg px-2.5 py-1 font-medium transition-colors",
                    approvalsFilter === "pending"
                      ? "bg-amber-500/15 text-amber-600 dark:text-amber-400 font-semibold"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {t("copilot.filterPending", "待审批")} ({pendingCount})
                </button>
                <button
                  type="button"
                  onClick={() => setApprovalsFilter("resolved")}
                  className={cn(
                    "rounded-lg px-2.5 py-1 font-medium transition-colors",
                    approvalsFilter === "resolved"
                      ? "bg-surface-hover text-foreground font-semibold"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {t("copilot.filterResolved", "已处理")} (
                  {pendingApprovals.length - pendingCount})
                </button>
              </div>
            </div>

            {/* Approvals List */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3.5">
              {filteredApprovals.length === 0 ? (
                <div className="py-12 text-center space-y-2">
                  <ShieldAlert className="mx-auto h-8 w-8 text-muted-foreground/60" />
                  <p className="text-sm font-medium text-foreground">
                    {t("copilot.noApprovals", "暂无待审批的高危操作")}
                  </p>
                  <p className="text-xs text-muted-foreground max-w-xs mx-auto">
                    {t(
                      "copilot.noApprovalsDesc",
                      "当外部 Agent 或自动化规则提议破坏性变更时，待审 Action Diff 卡片将在此显示。",
                    )}
                  </p>
                </div>
              ) : (
                filteredApprovals.map((action) => (
                  <ActionDiffCard key={action.id} action={action} />
                ))
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function PresetPromptCard({
  icon,
  title,
  desc,
  onClick,
}: {
  icon: React.ReactNode;
  title: string;
  desc: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-start gap-3 rounded-xl border border-border/70 bg-card p-3 text-left transition-all hover:border-primary/50 hover:bg-surface-hover/70 group"
    >
      <div className="mt-0.5 shrink-0 rounded-lg bg-well p-1.5 shadow-2xs group-hover:scale-105 transition-transform">
        {icon}
      </div>
      <div className="flex-1 min-w-0">
        <div className="text-xs font-semibold text-foreground group-hover:text-primary transition-colors">
          {title}
        </div>
        <div className="text-[11px] text-muted-foreground truncate mt-0.5">{desc}</div>
      </div>
    </button>
  );
}

function CopilotDrawerWithQuery() {
  const { data: aiStatus } = useAIStatusQuery();
  return <CopilotDrawerContent aiStatus={aiStatus} />;
}

export function CopilotDrawer() {
  const { isOpen } = useCopilotStore();
  if (!isOpen) return null;
  const queryClient = React.useContext(QueryClientContext);
  if (!queryClient) {
    return <CopilotDrawerContent aiStatus={{ configured: false, provider: "" }} />;
  }
  return <CopilotDrawerWithQuery />;
}

