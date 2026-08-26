import { useEffect, useMemo, useRef, useState, useCallback } from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import {
  Search,
  Sparkles,
  ArrowLeft,
  ArrowUp,
  Square,
  ChevronDown,
} from "lucide-react";
import { useTranslation } from "../i18n";
import { Area } from "./areas";
import { Action } from "../api";
import { CommandPaletteItem, mergeCommandItems } from "./globalActions";
import {
  translateAreaTitle,
  translateGroupLabel,
  translateNavItemLabel,
} from "./navI18n";
import { A2UIRenderer, A2UIWidget } from "../components/a2ui";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant" | "tool";
  content: string;
  thinking?: string;
  tokens?: number;
  toolCalls?: Array<A2UIWidget | { a2ui?: A2UIWidget; [key: string]: unknown }>;
}

export function ThinkingCollapsible({
  thinking,
  tokens,
  defaultOpen = false,
}: {
  thinking?: string;
  tokens?: number;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const { t } = useTranslation();

  if (!thinking) return null;

  return (
    <div
      role="region"
      aria-label={t("command.chat.thinking")}
      className="my-2 rounded-lg border border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.02] dark:bg-white/[0.02] overflow-hidden text-xs"
    >
      <button
        type="button"
        role="button"
        aria-expanded={open}
        onClick={() => setOpen((prev) => !prev)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setOpen((prev) => !prev);
          }
        }}
        className="flex w-full items-center justify-between px-3 py-1.5 text-left text-muted-foreground hover:bg-surface-hover/50 transition-colors font-medium cursor-pointer select-none"
      >
        <span className="flex items-center gap-1.5">
          <Sparkles className="h-3.5 w-3.5 text-primary shrink-0" />
          <span>{t("command.chat.thinking")}</span>
          {tokens !== undefined && (
            <span className="text-[10px] text-muted-foreground/70 font-mono">
              ({t("command.chat.tokens", { count: tokens })})
            </span>
          )}
        </span>
        <ChevronDown
          className={`h-3.5 w-3.5 shrink-0 transition-transform duration-200 ${open ? "rotate-180" : ""}`}
        />
      </button>
      {open && (
        <div className="border-t border-foreground/[0.06] dark:border-white/[0.06] p-2.5 font-mono text-[11px] leading-relaxed text-muted-foreground whitespace-pre-wrap max-h-48 overflow-y-auto">
          {thinking}
        </div>
      )}
    </div>
  );
}

export async function streamChatResponse({
  messages,
  signal,
  onThinking,
  onText,
  onTool,
  onError,
  onDone,
}: {
  messages: Array<{ role: string; content: string }>;
  signal?: AbortSignal;
  onThinking: (delta: string, tokens?: number) => void;
  onText: (delta: string) => void;
  onTool: (toolPayload: any) => void;
  onError: (err: string) => void;
  onDone: () => void;
}) {
  try {
    const res = await fetch("/api/ai/chat/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ messages }),
      signal,
    });

    if (!res.ok) {
      onError(`HTTP ${res.status}: ${res.statusText}`);
      return;
    }

    if (!res.body) {
      onError("No response body");
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    let currentEvent = "message";
    let dataLines: string[] = [];

    const flush = () => {
      if (dataLines.length === 0) return;
      const rawData = dataLines.join("\n");
      dataLines = [];
      let parsed: any = rawData;
      try {
        parsed = JSON.parse(rawData);
      } catch {
        // Fallback to raw string
      }

      if (currentEvent === "thinking") {
        const delta =
          typeof parsed === "object" && parsed !== null
            ? (parsed.delta ?? parsed.text ?? parsed.thinking ?? "")
            : String(parsed);
        const tokens =
          typeof parsed === "object" && parsed !== null ? parsed.tokens : undefined;
        onThinking(delta, tokens);
      } else if (currentEvent === "text" || currentEvent === "message") {
        const delta =
          typeof parsed === "object" && parsed !== null
            ? (parsed.delta ?? parsed.text ?? parsed.content ?? "")
            : String(parsed);
        onText(delta);
      } else if (currentEvent === "tool") {
        onTool(parsed);
      } else if (currentEvent === "done") {
        onDone();
      } else if (currentEvent === "error") {
        const err =
          typeof parsed === "object" && parsed !== null
            ? (parsed.error ?? parsed.message ?? JSON.stringify(parsed))
            : String(parsed);
        onError(err);
      }
    };

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("event:")) {
          flush();
          currentEvent = line.slice(6).trim();
        } else if (line.startsWith("data:")) {
          dataLines.push(line.slice(5).trimStart());
        } else if (line.trim() === "") {
          flush();
          currentEvent = "message";
        }
      }
    }
    flush();
    onDone();
  } catch (err: any) {
    if (err?.name === "AbortError") {
      return;
    }
    onError(err?.message || "Network error");
  }
}

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
            <>
              {/* Chat Header */}
              <div className="flex items-center justify-between border-b border-foreground/[0.08] dark:border-white/[0.08] bg-foreground/[0.015] dark:bg-white/[0.015] px-4 py-2.5 transition-colors">
                <button
                  type="button"
                  onClick={() => setMode("search")}
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
                    onChange={(e) => setChatInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        handleSend();
                      } else if (e.key === "Escape") {
                        e.preventDefault();
                        setMode("search");
                      }
                    }}
                    placeholder={t("command.chat.placeholder")}
                    aria-label={t("command.chat.inputLabel")}
                    className="flex-1 resize-none bg-transparent text-sm text-foreground placeholder:text-muted-foreground/60 outline-none border-none ring-0 focus:outline-none focus:border-none focus:ring-0 max-h-28 py-1.5 scrollbar-thin"
                  />
                  {isStreaming ? (
                    <button
                      type="button"
                      onClick={handleStop}
                      className="shrink-0 flex items-center gap-1 rounded-lg bg-muted/80 hover:bg-muted px-2.5 py-1.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                    >
                      <Square className="h-3.5 w-3.5" />
                      <span>{t("command.chat.stop")}</span>
                    </button>
                  ) : (
                    <button
                      type="button"
                      onClick={() => handleSend()}
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
          )}
        </BaseDialog.Popup>
      </BaseDialog.Portal>
    </BaseDialog.Root>
  );
}
