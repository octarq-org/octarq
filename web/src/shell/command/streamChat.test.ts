import { describe, expect, it, vi } from "vitest";
import { streamChatResponse } from "./streamChat";

const frame = (ev: Record<string, unknown>) => `data: ${JSON.stringify(ev)}\n\n`;

function stubFetch(body: string) {
  return vi.fn(
    async () =>
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new TextEncoder().encode(body));
            controller.close();
          },
        }),
        { status: 200, headers: { "Content-Type": "text/event-stream" } },
      ),
  );
}

function run(onDone: () => void, onError: (err: string) => void) {
  return streamChatResponse({
    messages: [{ role: "user", content: "hi" }],
    onThinking: vi.fn(),
    onText: vi.fn(),
    onTool: vi.fn(),
    onError,
    onDone,
  });
}

describe("streamChatResponse terminal events", () => {
  it("calls onDone once on RUN_FINISHED", async () => {
    vi.stubGlobal("fetch", stubFetch(frame({ type: "RUN_FINISHED" })));
    const onDone = vi.fn();
    await run(onDone, vi.fn());
    expect(onDone).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
  });

  it("does not call onDone after RUN_ERROR", async () => {
    vi.stubGlobal("fetch", stubFetch(frame({ type: "RUN_ERROR", message: "boom", code: "x" })));
    const onDone = vi.fn();
    const onError = vi.fn();
    await run(onDone, onError);
    expect(onError).toHaveBeenCalledWith("boom", { status: 0, code: "x" });
    expect(onDone).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});
