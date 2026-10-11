import React, { useState } from "react";
import { Button, cn, LockedFeature } from "@octarq/plugin-sdk";
import {
  Sparkles,
  Send,
  Square,
  ChevronDown,
  ChevronRight,
  Bot,
  Flame,
  CreditCard,
  Globe2,
  FileCheck,
} from "lucide-react";
import { useTranslation } from "../i18n";
import { useCopilotStore } from "./store";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { ActionDiffCard } from "./ActionDiffCard";
import type { AIStatus } from "./types";

export interface CopilotChatTabProps {
  aiStatus?: AIStatus;
  messagesEndRef: React.RefObject<HTMLDivElement>;
  inputRef: React.RefObject<HTMLTextAreaElement>;
  streamLocked: boolean;
  onSendMessage: (text?: string) => Promise<void>;
  onStopGenerating: () => void;
}

export function CopilotChatTab({
  aiStatus,
  messagesEndRef,
  inputRef,
  streamLocked,
  onSendMessage,
  onStopGenerating,
}: CopilotChatTabProps) {
  const { t } = useTranslation();
  const { messages, input, setInput, isStreaming, setActiveTab } = useCopilotStore();
  const [thinkingExpanded, setThinkingExpanded] = useState<Record<string, boolean>>({});

  const aiUnconfigured = aiStatus != null && !aiStatus.configured;

  const toggleThinking = (msgId: string) => {
    setThinkingExpanded((prev) => ({ ...prev, [msgId]: !prev[msgId] }));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      onSendMessage();
    }
  };

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Messages Area */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {aiUnconfigured && (
          <div
            role="alert"
            data-testid="copilot-unconfigured-card"
            className="rounded-2xl border border-warning-fg/30 bg-warning-fg/10 p-3.5 text-xs leading-relaxed"
          >
            <div className="font-bold text-warning-fg">
              {t("copilot.unconfiguredTitle", "未配置 LLM，请前往设置")}
            </div>
            <div className="mt-1 text-muted-foreground">
              {t(
                "copilot.unconfiguredDesc",
                "在「Inbox AI → Configure」中添加 LLM 提供商后即可开始对话。配置缺失时不会生成任何模拟回复。",
              )}
            </div>
          </div>
        )}
        {streamLocked && (
          <div data-testid="copilot-locked-feature">
            <LockedFeature status={402} feature="AI Copilot" />
          </div>
        )}
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
                  icon={<Flame className="h-4 w-4 text-warning-fg" />}
                  title={t("copilot.promptLinks", "汇总本周点击最高的短链")}
                  desc={t("copilot.promptLinksDesc", "统计近 7 天访问趋势与热门重定向排行")}
                  onClick={() => onSendMessage(t("copilot.promptLinks", "汇总本周点击最高的短链"))}
                />
                <PresetPromptCard
                  icon={<CreditCard className="h-4 w-4 text-success-fg" />}
                  title={t("copilot.promptPayment", "查询最近一笔支付状态")}
                  desc={t("copilot.promptPaymentDesc", "核对 Stripe Webhook 履约与证书签发记录")}
                  onClick={() => onSendMessage(t("copilot.promptPayment", "查询最近一笔支付状态"))}
                />
                <PresetPromptCard
                  icon={<Globe2 className="h-4 w-4 text-accent-fg" />}
                  title={t("copilot.promptDns", "排查 api.octarq.com 的 DNS 异常")}
                  desc={t("copilot.promptDnsDesc", "检查权威解析节点延迟与 A/CNAME 配置一致性")}
                  onClick={() => onSendMessage(t("copilot.promptDns", "排查 api.octarq.com 的 DNS 异常"))}
                />
                <PresetPromptCard
                  icon={<FileCheck className="h-4 w-4 text-accent-fg" />}
                  title={t("copilot.promptApprovals", "检查待处理的审批操作")}
                  desc={t("copilot.promptApprovalsDesc", "打开待审队列，核对智能体发起的高危变更")}
                  onClick={() => setActiveTab("approvals")}
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
        <div className="relative flex flex-col rounded-2xl border border-border bg-background focus-within:border-primary/50 focus-within:ring-1 focus-within:ring-primary/50 transition-colors">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t("copilot.inputPlaceholder", "问点什么，或下达运营指令 (Enter 发送)…")}
            aria-label={t("copilot.inputPlaceholder", "问点什么，或下达运营指令 (Enter 发送)…")}
            rows={2}
            className="w-full resize-none bg-transparent p-3 text-xs sm:text-sm text-foreground placeholder:text-muted-foreground outline-none focus-visible:outline-none"
          />

          <div className="flex items-center justify-between border-t border-border/40 px-3 py-2 bg-well/20 rounded-b-2xl">
            <div className="text-[10px] text-muted-foreground hidden sm:block">
              {t("copilot.shortcutHint", "Shift + Enter 换行 · Enter 发送 · ⌘&nbsp;J 唤起/收起")}
            </div>

            <div className="flex items-center gap-1.5 ml-auto">
              {isStreaming ? (
                <Button
                  variant="danger"
                  size="sm"
                  onClick={onStopGenerating}
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
                  onClick={() => onSendMessage()}
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
      className="flex items-start gap-3 rounded-xl border border-border/70 bg-card p-3 text-left transition-colors hover:border-primary/50 hover:bg-surface-hover/70 group focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
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
