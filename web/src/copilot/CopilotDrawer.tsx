import { cn } from "@octarq/plugin-sdk";
import React, { useEffect, useRef, useState } from "react";
import { Sparkles, X, Trash2, ShieldAlert, MessageSquare } from "lucide-react";
import { useTranslation } from "../i18n";
import { useCopilotStore } from "./store";
import { AIStreamError, fetchApprovals, useAIStatusQuery, streamAIChat } from "./api";
import type { ActionDiff, ChatMessage, AIStatus } from "./types";
import { QueryClientContext } from "@tanstack/react-query";
import { CopilotChatTab } from "./CopilotChatTab";
import { CopilotApprovalsTab } from "./CopilotApprovalsTab";

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
    setApprovals,
    setApprovalsError,
  } = useCopilotStore();

  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const [streamLocked, setStreamLocked] = useState(false);
  const [approvalsLoading, setApprovalsLoading] = useState(false);

  const pendingCount = pendingApprovals.filter((a) => a.status === "pending").length;

  // Approvals are the backend agent_approvals truth — loaded on tab open,
  // never seeded locally. Fail-Closed: fetch failures surface as an error
  // state (or the 402 upsell), never as placeholder cards.
  const refreshApprovals = async () => {
    setApprovalsLoading(true);
    try {
      const rows = await fetchApprovals();
      setApprovals(rows);
    } catch (err) {
      if (err instanceof AIStreamError && err.code === "locked") {
        setApprovalsError("locked");
      } else {
        setApprovalsError(err instanceof Error ? err.message : "load failed");
      }
    } finally {
      setApprovalsLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen && activeTab === "approvals") {
      void refreshApprovals();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen, activeTab]);

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
        const raw = toolCall.args?.actionDiff as ActionDiff | undefined;
        if (raw && typeof raw.approvalId === "string" && raw.approvalId) {
          addActionDiff(raw);
          updateLastMessage((prev) => ({
            ...prev,
            actionDiffs: [...(prev.actionDiffs || []), raw],
          }));
        }
      },
      onError: (err) => {
        if (err instanceof AIStreamError && err.code === "locked") {
          setStreamLocked(true);
        }
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

  if (!isOpen) return null;

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
        className="relative z-50 flex h-full w-full max-w-lg flex-col border-l border-border bg-background shadow-2xl transition-[transform,opacity] duration-200 animate-in slide-in-from-right"
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-4 py-3 bg-well/40">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl brand-gradient text-white shadow-xs">
              <Sparkles className="h-4 w-4" aria-hidden="true" />
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
                  <span className="inline-flex items-center gap-1 rounded-full bg-warning-fg/10 px-2 py-0.5 text-[10px] font-semibold text-warning-fg">
                    <span className="h-1.5 w-1.5 rounded-full bg-warning-fg" />
                    <span>{t("copilot.unconfigured", "未配置")}</span>
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
          <CopilotChatTab
            aiStatus={aiStatus}
            messagesEndRef={messagesEndRef}
            inputRef={inputRef}
            streamLocked={streamLocked}
            onSendMessage={handleSendMessage}
            onStopGenerating={handleStopGenerating}
          />
        )}

        {/* Tab Content: Approvals */}
        {activeTab === "approvals" && (
          <CopilotApprovalsTab
            onRefreshApprovals={refreshApprovals}
            approvalsLoading={approvalsLoading}
          />
        )}
      </div>
    </div>
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
