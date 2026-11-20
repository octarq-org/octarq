import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { ChevronDown, Search, Sparkles } from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { useTranslation } from "../i18n";
import { Area } from "./areas";
import { Action } from "../api";
import { CommandPaletteItem, mergeCommandItems } from "./globalActions";
import {
  translateAreaTitle,
  translateGroupLabel,
  translateNavItemLabel,
} from "./navI18n";
import { type ChatMessage } from "./command/types";
import { ThinkingCollapsible } from "./command/ThinkingCollapsible";
import { groupCommandItems } from "./command/groupItems";
import { newChatRunId, newChatThreadId, streamChatResponse } from "./command/streamChat";
import { ChatView } from "./command/ChatView";
import { agentFailureText } from "./agentFailureText";

export { type ChatMessage } from "./command/types";
export { ThinkingCollapsible } from "./command/ThinkingCollapsible";
export { streamChatResponse } from "./command/streamChat";

export function CommandPalette({
  open,
  onClose,
  areas,
  settingsArea,
  onNavigate,
  actions = [],
}: {
  open: boolean;
  onClose: () => void;
  areas: Area[];
  settingsArea: Area;
  onNavigate: (path: string) => void;
  actions?: Action[];
}) {
  const [mode, setMode] = useState<"search" | "chat">("search");
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatInput, setChatInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [chatError, setChatError] = useState<string | null>(null);

  const inputRef = useRef<HTMLInputElement>(null);
  const chatInputRef = useRef<HTMLTextAreaElement>(null);
  const chatScrollRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const abortControllerRef = useRef<AbortController | null>(null);
  const threadIdRef = useRef(newChatThreadId());

  const { t } = useTranslation();

  const navItems = useMemo<CommandPaletteItem[]>(
    () =>
      [...areas, settingsArea].flatMap((a) =>
        a.groups.flatMap((g) =>
          g.items.flatMap((i) => {
            if (i.external) return [];
            const areaTitle = translateAreaTitle(t, a.id, a.title);
            const groupLabel = translateGroupLabel(t, g.label);
            return [
              {
                id: i.path,
                label: translateNavItemLabel(t, i.id, i.label),
                area: areaTitle,
                group: groupLabel,
                path: i.path,
                Icon: i.Icon,
                iconStr: i.iconStr,
                isAction: false,
              },
            ];
          }),
        ),
      ),
    [areas, settingsArea, t],
  );

  const commands = useMemo(
    () =>
      mergeCommandItems(
        actions.map((a) => ({ ...a, label: translateNavItemLabel(t, a.id, a.label) })),
        navItems,
        {
          includeBuiltin: true,
          t,
          onNavigate,
        },
      ),
    [actions, navItems, onNavigate, t],
  );

  const areaTitles = useMemo(
    () => [...areas, settingsArea].map((a) => translateAreaTitle(t, a.id, a.title)),
    [areas, settingsArea, t],
  );

  const askAiItem = useMemo<CommandPaletteItem | null>(() => {
    const trimmed = q.trim();
    if (!trimmed) return null;
    return {
      id: "ask-ai-entry",
      label: `${t("command.chat.askAi")}: “${trimmed}”`,
      path: "",
      isAction: true,
      Icon: Sparkles,
      area: t("command.chat.modeBadge"),
      group: "AI",
    };
  }, [q, t]);

  const isSearching = q.trim().length > 0;

  const matchedItems = useMemo(() => {
    const nonDocCommands = commands.filter(
      (c) => !c.path.startsWith("/help") && !c.path.startsWith("/admin/help"),
    );
    const needle = q.trim().toLowerCase();
    if (!needle) return nonDocCommands;
    return nonDocCommands.filter(
      (c) =>
        c.label.toLowerCase().includes(needle) ||
        (c.isAction ? c.category?.toLowerCase().includes(needle) : false) ||
        (!c.isAction && c.area?.toLowerCase().includes(needle)) ||
        (!c.isAction && c.group?.toLowerCase().includes(needle)) ||
        c.path.toLowerCase().includes(needle) ||
        (c.actionType ? c.actionType.toLowerCase().includes(needle) : false) ||
        c.id.toLowerCase().includes(needle),
    );
  }, [q, commands]);

  const grouped = useMemo(
    () => groupCommandItems(matchedItems, t("command.quickActions", "Quick Actions"), areaTitles),
    [matchedItems, areaTitles, t],
  );

  const { sections, visibleItems, askAiIndex } = useMemo(() => {
    const nextVisible: CommandPaletteItem[] = [];
    const nextSections = grouped.map((g) => {
      const collapsed = !isSearching && !!collapsedGroups[g.key];
      const startIndex = nextVisible.length;
      if (!collapsed) {
        nextVisible.push(...g.items);
      }
      return { ...g, collapsed, startIndex };
    });
    let nextAskAiIndex = -1;
    if (askAiItem) {
      nextAskAiIndex = nextVisible.length;
      nextVisible.push(askAiItem);
    }
    return { sections: nextSections, visibleItems: nextVisible, askAiIndex: nextAskAiIndex };
  }, [grouped, collapsedGroups, isSearching, askAiItem]);

  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
      setCollapsedGroups({});
      setMode("search");
      setChatError(null);
    } else {
      abortControllerRef.current?.abort();
      setIsStreaming(false);
    }
  }, [open]);

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort();
    };
  }, []);

  useEffect(() => {
    setSel(0);
  }, [q]);

  useEffect(() => {
    setSel((s) => Math.min(s, Math.max(0, visibleItems.length - 1)));
  }, [visibleItems.length]);

  useEffect(() => {
    if (open && mode === "search") {
      itemRefs.current[sel]?.scrollIntoView?.({ block: "nearest" });
    }
  }, [sel, open, mode]);

  useEffect(() => {
    if (mode === "chat") {
      chatScrollRef.current?.scrollTo({
        top: chatScrollRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [messages, isStreaming, mode]);

  useEffect(() => {
    if (mode === "chat") {
      setTimeout(() => chatInputRef.current?.focus(), 50);
    } else if (open) {
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [mode, open]);

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort();
    setIsStreaming(false);
  }, []);

  const handleSend = useCallback(
    async (promptOverride?: string) => {
      const prompt = (promptOverride !== undefined ? promptOverride : chatInput).trim();
      if (!prompt || isStreaming) return;

      setChatInput("");
      setChatError(null);

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

      const outboundMessages = [...newHistory].map((m) => ({
        role: m.role,
        content: m.content,
      }));

      await streamChatResponse({
        messages: outboundMessages,
        threadId: threadIdRef.current,
        runId: newChatRunId(),
        signal: controller.signal,
        onThinking: (delta, tokens) => {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === asstMsgId
                ? {
                    ...m,
                    thinking: (m.thinking || "") + delta,
                    tokens: tokens !== undefined ? tokens : m.tokens,
                  }
                : m,
            ),
          );
        },
        onText: (delta) => {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === asstMsgId
                ? {
                    ...m,
                    content: m.content + delta,
                  }
                : m,
            ),
          );
        },
        onTool: (toolPayload) => {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === asstMsgId
                ? {
                    ...m,
                    toolCalls: [...(m.toolCalls || []), toolPayload],
                  }
                : m,
            ),
          );
        },
        onError: (err, failure) => {
          setChatError(agentFailureText(t, failure, err));
          setIsStreaming(false);
        },
        onDone: () => {
          setIsStreaming(false);
        },
      });
    },
    [chatInput, isStreaming, messages, t],
  );

  const startChatWithQuery = useCallback(
    (query: string) => {
      setMode("chat");
      setQ("");
      if (query.trim()) {
        handleSend(query.trim());
      }
    },
    [handleSend],
  );

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    if (val.startsWith("/") || val.startsWith("?")) {
      const initialPrompt = val.slice(1);
      setQ("");
      setMode("chat");
      setChatInput(initialPrompt);
      return;
    }
    setQ(val);
  };

  const executeItem = useCallback(
    (c: CommandPaletteItem) => {
      if (c.id === "ask-ai-entry") {
        startChatWithQuery(q);
      } else if (c.actionHandler) {
        c.actionHandler();
        onClose();
      } else {
        onNavigate(c.path);
      }
    },
    [startChatWithQuery, q, onClose, onNavigate],
  );

  const toggleGroup = (key: string) => {
    setCollapsedGroups((prev) => ({ ...prev, [key]: !prev[key] }));
  };

  const onSearchKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(s + 1, Math.max(0, visibleItems.length - 1)));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(s - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const c = visibleItems[sel];
      if (c) executeItem(c);
    }
  };

  const renderCandidateRow = (c: CommandPaletteItem, flatIndex: number) => {
    const active = flatIndex === sel;
    const badge =
      c.id === "ask-ai-entry"
        ? t("command.chat.modeBadge")
        : c.shortcut
        ? c.shortcut
        : c.isAction
        ? t("command.create", "Create")
        : c.group;

    return (
      <button
        key={c.id}
        ref={(el) => {
          itemRefs.current[flatIndex] = el;
        }}
        type="button"
        onMouseEnter={() => setSel(flatIndex)}
        onClick={() => executeItem(c)}
        className={cn(
          "flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-left transition-colors duration-100",
          active
            ? "bg-accent-soft font-medium text-accent-fg dark:bg-primary/20"
            : "text-foreground hover:bg-surface-hover/80",
        )}
      >
        {c.iconStr ? (
          <span className="w-4 text-center text-sm">{c.iconStr}</span>
        ) : c.Icon ? (
          <c.Icon
            className={cn("h-4 w-4 shrink-0 transition-colors", active ? "text-accent-fg" : "text-muted-foreground")}
            strokeWidth={1.75}
          />
        ) : null}
        <span className="flex-1 truncate text-[13px]">{c.label}</span>
        {badge && (
          <span className="shrink-0 rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
            {badge}
          </span>
        )}
      </button>
    );
  };

  return (
    <BaseDialog.Root
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          handleStop();
          onClose();
        }
      }}
    >
      <BaseDialog.Portal>
        <BaseDialog.Backdrop className="fixed inset-0 z-[100] bg-black/50 backdrop-blur-sm modal-overlay" />
        <BaseDialog.Popup
          initialFocus={mode === "search" ? inputRef : chatInputRef}
          aria-label={mode === "search" ? t("command.placeholder") : t("command.chat.modeBadge")}
          className="glass-strong fixed left-1/2 top-[12vh] z-[100] w-[calc(100%-2rem)] max-w-xl -translate-x-1/2 overflow-hidden rounded-lg modal-card outline-none"
        >
          {mode === "search" ? (
            <>
              <div className="flex items-center gap-3 border-b border-border focus-within:border-primary/50 bg-well px-4 py-1 transition-colors">
                <Search className="h-4 w-4 shrink-0 text-accent-fg" />
                <input
                  ref={inputRef}
                  value={q}
                  onChange={handleSearchChange}
                  onKeyDown={onSearchKey}
                  placeholder={t("command.placeholder")}
                  className="w-full bg-transparent py-3 text-sm font-medium text-foreground placeholder:text-muted-foreground/60 outline-none border-none ring-0 focus:outline-none focus:border-none focus:ring-0 focus:shadow-none focus-visible:outline-none focus-visible:ring-0"
                />
                <button
                  type="button"
                  onClick={() => setMode("chat")}
                  className="flex items-center gap-1 rounded-md border border-primary/20 bg-accent-soft hover:bg-primary/20 px-2 py-1 text-xs font-medium text-accent-fg transition-colors shrink-0 cursor-pointer"
                  aria-label={t("command.chat.askAi")}
                >
                  <Sparkles className="h-3.5 w-3.5" />
                  <span>{t("command.chat.askAi")}</span>
                </button>
                <kbd className="shrink-0 rounded-md border border-border bg-muted px-1.5 py-0.5 text-[10px] font-mono font-medium text-muted-foreground">
                  esc
                </kbd>
              </div>
              <div className="max-h-[50vh] overflow-y-auto p-2 scrollbar-thin">
                {sections.length === 0 && !askAiItem ? (
                  <div className="px-3 py-8 text-center">
                    <p className="text-sm text-muted-foreground">
                      {t("command.emptyTitle")} <span className="font-mono">{`“${q}”`}</span>
                    </p>
                    <p className="mx-auto mt-1.5 max-w-sm text-xs leading-relaxed text-muted-foreground/70">
                      {t("command.emptyHint", { areas: areaTitles.join(", ") })}
                    </p>
                  </div>
                ) : (
                  <div className="space-y-1">
                    {sections.map((section) => (
                      <div key={section.key} data-slot="command-group">
                        <button
                          type="button"
                          aria-expanded={!section.collapsed}
                          onClick={() => toggleGroup(section.key)}
                          className="flex w-full cursor-pointer items-center justify-between gap-2 rounded-md px-2 py-1.5 text-[11px] font-semibold text-muted-foreground transition-colors duration-100 hover:bg-surface-hover hover:text-foreground"
                        >
                          <span className="flex items-center gap-1.5 truncate">
                            <ChevronDown
                              className={cn(
                                "h-3.5 w-3.5 shrink-0 transition-transform duration-150",
                                section.collapsed && "-rotate-90",
                              )}
                              strokeWidth={1.75}
                            />
                            <span className="truncate">{section.label}</span>
                          </span>
                          <span className="font-mono text-[10px] font-medium text-muted-foreground/75">
                            {section.items.length}
                          </span>
                        </button>
                        {!section.collapsed && (
                          <div
                            data-slot="command-group-items"
                            className="mb-1 ml-3.5 mt-0.5 space-y-0.5 border-l border-border/70 pl-2"
                          >
                            {section.items.map((item, idx) =>
                              renderCandidateRow(item, section.startIndex + idx),
                            )}
                          </div>
                        )}
                      </div>
                    ))}
                    {askAiItem && askAiIndex >= 0 && (
                      <div className={cn(sections.length > 0 && "mt-1.5 border-t border-border pt-1.5")}>
                        {renderCandidateRow(askAiItem, askAiIndex)}
                      </div>
                    )}
                  </div>
                )}
              </div>
            </>
          ) : (
            <ChatView
              messages={messages}
              chatInput={chatInput}
              isStreaming={isStreaming}
              chatError={chatError}
              chatScrollRef={chatScrollRef}
              chatInputRef={chatInputRef}
              onChatInputChange={setChatInput}
              onSend={() => handleSend()}
              onStop={handleStop}
              onBackToSearch={() => setMode("search")}
            />
          )}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
