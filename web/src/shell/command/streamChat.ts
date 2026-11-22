import type { ToolPayload } from "./types";

export interface ChatFailure {
  status: number;
  code?: string;
}

export function newChatThreadId(): string {
  return withRandom("thread");
}

export function newChatRunId(): string {
  return withRandom("run");
}

function withRandom(prefix: string): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid) return `${prefix}-${uuid}`;
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

interface ChatStreamEvent {
  type?: string;
  delta?: unknown;
  message?: unknown;
  code?: unknown;
  toolCallId?: string;
  toolCallName?: string;
}

export async function streamChatResponse({
  messages,
  threadId,
  runId,
  accountId,
  signal,
  onThinking,
  onText,
  onTool,
  onError,
  onDone,
}: {
  messages: Array<{ role: string; content: string }>;
  threadId?: string;
  runId?: string;
  accountId?: string;
  signal?: AbortSignal;
  onThinking: (delta: string, tokens?: number) => void;
  onText: (delta: string) => void;
  onTool: (toolPayload: ToolPayload) => void;
  onError: (err: string, failure?: ChatFailure) => void;
  onDone: () => void;
}) {
  try {
    const res = await fetch("/api/ai/chat/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
      body: JSON.stringify({
        threadId: threadId ?? newChatThreadId(),
        runId: runId ?? newChatRunId(),
        messages,
        ...(accountId ? { accountId } : {}),
      }),
      signal,
    });

    if (!res.ok) {
      let message = `HTTP ${res.status}: ${res.statusText}`;
      let code: string | undefined;
      try {
        const body = (await res.json()) as { code?: string; message?: string; detail?: string };
        if (typeof body.code === "string") code = body.code;
        if (typeof body.message === "string" && body.message) message = body.message;
        else if (typeof body.detail === "string" && body.detail) message = body.detail;
      } catch {
      }
      onError(message, { status: res.status, code });
      return;
    }

    if (!res.body) {
      onError("No response body");
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let dataLines: string[] = [];
    let settled = false;

    const calls = new Map<string, { name: string; args: string }>();

    const flush = () => {
      if (dataLines.length === 0) return;
      const raw = dataLines.join("\n");
      dataLines = [];

      let event: ChatStreamEvent;
      try {
        event = JSON.parse(raw) as ChatStreamEvent;
      } catch {
        return;
      }

      switch (event.type) {
        case "THINKING_TEXT_MESSAGE_CONTENT":
          onThinking(String(event.delta ?? ""));
          return;
        case "TEXT_MESSAGE_CONTENT":
          onText(String(event.delta ?? ""));
          return;
        case "TOOL_CALL_START":
          if (event.toolCallId) {
            calls.set(event.toolCallId, { name: String(event.toolCallName ?? ""), args: "" });
          }
          return;
        case "TOOL_CALL_ARGS": {
          const call = event.toolCallId ? calls.get(event.toolCallId) : undefined;
          if (call) call.args += String(event.delta ?? "");
          return;
        }
        case "TOOL_CALL_RESULT": {
          const call = event.toolCallId ? calls.get(event.toolCallId) : undefined;
          if (event.toolCallId) calls.delete(event.toolCallId);
          if (call) onTool(toolPayload(event.toolCallId ?? "", call.name, call.args));
          return;
        }
        case "RUN_ERROR": {
          const code = typeof event.code === "string" ? event.code : undefined;
          settled = true;
          onError(String(event.message ?? "Request failed"), { status: 0, code });
          return;
        }
        case "RUN_FINISHED":
          settled = true;
          onDone();
          return;
        default:
          return;
      }
    };

    while (true) {
      const { done: streamDone, value } = await reader.read();
      if (streamDone) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("data:")) {
          dataLines.push(line.slice(5).trimStart());
        } else if (line.trim() === "") {
          flush();
        }
      }
    }
    flush();
    if (!settled) onDone();
  } catch (err: unknown) {
    if (err && typeof err === "object" && "name" in err && err.name === "AbortError") {
      return;
    }
    const msg = err instanceof Error ? err.message : "Network error";
    onError(msg);
  }
}

function toolPayload(id: string, name: string, rawArgs: string): ToolPayload {
  let args: unknown = {};
  try {
    args = rawArgs ? JSON.parse(rawArgs) : {};
  } catch {
    args = { args: rawArgs };
  }
  if (args && typeof args === "object" && !Array.isArray(args)) {
    return { ...(args as Record<string, unknown>), id, name } as ToolPayload;
  }
  return { id, name, args } as ToolPayload;
}
