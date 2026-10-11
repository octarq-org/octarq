import type { ToolPayload } from "./types";

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
  onTool: (toolPayload: ToolPayload) => void;
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
      let parsed: unknown = rawData;
      try {
        parsed = JSON.parse(rawData);
      } catch {
        // Fallback to raw string
      }

      const parsedObj = typeof parsed === "object" && parsed !== null ? (parsed as Record<string, unknown>) : null;

      if (currentEvent === "thinking") {
        const delta = parsedObj
          ? String(parsedObj.delta ?? parsedObj.text ?? parsedObj.thinking ?? "")
          : String(parsed);
        const tokens = typeof parsedObj?.tokens === "number" ? parsedObj.tokens : undefined;
        onThinking(delta, tokens);
      } else if (currentEvent === "text" || currentEvent === "message") {
        const delta = parsedObj
          ? String(parsedObj.delta ?? parsedObj.text ?? parsedObj.content ?? "")
          : String(parsed);
        onText(delta);
      } else if (currentEvent === "tool") {
        onTool(parsed as ToolPayload);
      } else if (currentEvent === "done") {
        onDone();
      } else if (currentEvent === "error") {
        const err = parsedObj
          ? String(parsedObj.error ?? parsedObj.message ?? JSON.stringify(parsed))
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
  } catch (err: unknown) {
    if (err && typeof err === "object" && "name" in err && err.name === "AbortError") {
      return;
    }
    const msg = err instanceof Error ? err.message : "Network error";
    onError(msg);
  }
}
