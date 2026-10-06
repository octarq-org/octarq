import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { Search, Sparkles } from "lucide-react";
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
import { streamChatResponse } from "./command/streamChat";
import { ChatView } from "./command/ChatView";

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
  // Admin-filtered merged Settings area with plugin-contributed settings pages.
  settingsArea: Area;
  onNavigate: (path: string) => void;
  actions?: Action[];
}) {
  const [mode, setMode] = useState<"search" | "chat">("search");
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatInput, setChatInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [chatError, setChatError] = useState<string | null>(null);

  const inputRef = useRef<HTMLInputElement>(null);
  const chatInputRef = useRef<HTMLTextAreaElement>(null);
  const chatScrollRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const { t } = useTranslation();

  // Flatten nav items (areas + settings) with translated labels for search.
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
      ),
    [actions, navItems, t],
  );

  const areaTitles = useMemo(
    () => [...areas, settingsArea].map((a) => translateAreaTitle(t, a.id, a.title)),
    [areas, settingsArea, t],
  );

  // When q is non-empty, we can prepend an "Ask AI" action item to search results
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

  const filtered = useMemo(() => {
    const nonDocCommands = commands.filter(
      (c) => !c.path.startsWith("/help") && !c.path.startsWith("/admin/help"),
    );
    const needle = q.trim().toLowerCase();
    let result = nonDocCommands;
    if (needle) {
      result = nonDocCommands.filter(
        (c) =>
          c.label.toLowerCase().includes(needle) ||
          (c.isAction ? c.category?.toLowerCase().includes(needle) : false) ||
          (!c.isAction && c.area?.toLowerCase().includes(needle)) ||
          (!c.isAction && c.group?.toLowerCase().includes(needle)) ||
          c.path.toLowerCase().includes(needle),
      );
    }
    if (askAiItem) {
      return [askAiItem, ...result];
    }
    return result;
  }, [q, commands, askAiItem]);

  // Clean up streaming on unmount or dialog close
  useEffect(() => {
    if (open) {
      setQ("");
      setSel(0);
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

  // Auto-scroll chat to bottom
  useEffect(() => {
    if (mode === "chat") {
      chatScrollRef.current?.scrollTo({
        top: chatScrollRef.current.scrollHeight,
        behavior: "smooth",
      });
    }
  }, [messages, isStreaming, mode]);

  // Focus textarea when switching to chat mode
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
        onError: (err) => {
          setChatError(err);
          setIsStreaming(false);
        },
        onDone: () => {
          setIsStreaming(false);
        },
      });
    },
    [chatInput, isStreaming, messages],
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

  // Search input change handler — detects / or ? prefixes
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

  // Arrow/Enter drive search result list navigation.
  const onSearchKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSel((s) => Math.min(s + 1, filtered.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSel((s) => Math.max(s - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const c = filtered[sel];
      if (c) {
        if (c.id === "ask-ai-entry") {
          startChatWithQuery(q);
        } else {
          onNavigate(c.path);
        }
      }
    }
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
              <div className="flex items-center gap-3 border-b border-foreground/[0.08] dark:border-white/[0.08] focus-within:border-primary/50 bg-foreground/[0.015] dark:bg-white/[0.015] px-4 py-1 transition-colors">
                <Search className="h-4 w-4 shrink-0 text-primary" />
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
                  className="flex items-center gap-1 rounded-md border border-primary/20 bg-primary/10 hover:bg-primary/20 px-2 py-1 text-xs font-medium text-primary transition-colors shrink-0 cursor-pointer"
                  aria-label={t("command.chat.askAi")}
                >
                  <Sparkles className="h-3.5 w-3.5" />
                  <span>{t("command.chat.askAi")}</span>
                </button>
                <kbd className="shrink-0 rounded-md border border-foreground/10 dark:border-white/10 bg-muted/60 px-1.5 py-0.5 text-[10px] font-mono font-medium text-muted-foreground">
                  esc
                </kbd>
              </div>
              <div className="max-h-[50vh] overflow-y-auto p-2 scrollbar-thin">
                {filtered.length === 0 ? (
                  <div className="px-3 py-8 text-center">
                    <p className="text-sm text-muted-foreground">
                      {t("command.emptyTitle")} <span className="font-mono">{`“${q}”`}</span>
                    </p>
                    <p className="mx-auto mt-1.5 max-w-sm text-xs leading-relaxed text-muted-foreground/70">
                      {t("command.emptyHint", { areas: areaTitles.join(", ") })}
                    </p>
                  </div>
                ) : (
                  filtered.map((c, i) => (
                    <button
                      key={c.id}
                      onMouseEnter={() => setSel(i)}
                      onClick={() => {
                        if (c.id === "ask-ai-entry") {
                          startChatWithQuery(q);
                        } else {
                          onNavigate(c.path);
                        }
                      }}
                      className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-all ${
                        i === sel
                          ? "bg-primary/10 text-primary dark:bg-primary/20 font-medium"
                          : "hover:bg-surface-hover/80 text-foreground/90"
                      }`}
                    >
                      {c.iconStr ? (
                        <span className="w-4 text-center text-sm">{c.iconStr}</span>
                      ) : c.Icon ? (
                        <c.Icon
                          className={`h-4 w-4 shrink-0 transition-colors ${
                            i === sel ? "text-primary" : "text-muted-foreground"
                          }`}
                          strokeWidth={1.75}
                        />
                      ) : null}
                      <span className="flex-1 truncate text-sm">{c.label}</span>
                      <span className="shrink-0 rounded-md border border-foreground/5 dark:border-white/5 bg-muted/40 dark:bg-white/5 px-2 py-0.5 text-[10px] font-mono text-muted-foreground">
                        {c.id === "ask-ai-entry"
                          ? t("command.chat.modeBadge")
                          : c.isAction
                          ? t("command.create", "Create")
                          : `${c.area} · ${c.group}`}
                      </span>
                    </button>
                  ))
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
