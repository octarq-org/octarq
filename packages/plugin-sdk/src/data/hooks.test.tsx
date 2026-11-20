// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePluginData, usePluginMutation, usePluginStream } from "./hooks";
import { ApiError, resetPluginHttp } from "./request";

function jsonResponse(body: unknown, init: { status?: number } = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "content-type": "application/json" },
  });
}

function withClient(ui: React.ReactNode, client = new QueryClient({ defaultOptions: { queries: { retry: false } } })) {
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  resetPluginHttp();
  fetchMock = vi.fn(async () => jsonResponse({ n: 1 }));
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("usePluginData", () => {
  it("reads the route and exposes the value", async () => {
    function Probe() {
      const { data } = usePluginData<{ n: number }>(["thing"], "/api/x/thing");
      return <div data-testid="value">{data ? data.n : "pending"}</div>;
    }
    withClient(<Probe />);
    expect(await screen.findByText("1")).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("shares one request between two consumers of the same key", async () => {
    function Probe({ label }: { label: string }) {
      const { data } = usePluginData<{ n: number }>(["shared"], "/api/x/thing");
      return <div>{`${label}:${data ? data.n : "pending"}`}</div>;
    }
    withClient(
      <>
        <Probe label="a" />
        <Probe label="b" />
      </>,
    );
    expect(await screen.findByText("a:1")).toBeTruthy();
    expect(await screen.findByText("b:1")).toBeTruthy();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the status of a failed read", async () => {
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ message: "locked" }), { status: 402 }));
    function Probe() {
      const { error } = usePluginData(["thing"], "/api/x/thing");
      return <div data-testid="error">{error ? `${(error as ApiError).status}` : "none"}</div>;
    }
    withClient(<Probe />);
    expect(await screen.findByText("402")).toBeTruthy();
  });
});

describe("usePluginMutation", () => {
  it("sends the body, then invalidates the listed keys", async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: 1 }));
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    function Probe() {
      const mutation = usePluginMutation<{ name: string }, { id: number }>({
        method: "POST",
        path: "/api/x/things",
        body: (vars) => ({ name: vars.name }),
        invalidate: [["things"]],
      });
      return (
        <button type="button" onClick={() => mutation.mutate({ name: "alpha" })}>
          {mutation.data ? `id:${mutation.data.id}` : "idle"}
        </button>
      );
    }
    withClient(<Probe />, client);
    const button = await screen.findByRole("button");
    act(() => button.click());
    expect(await screen.findByText("id:1")).toBeTruthy();
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ name: "alpha" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["things"] });
  });

  it("derives the route from the variables", async () => {
    function Probe() {
      const mutation = usePluginMutation<{ id: number }>({
        method: "DELETE",
        path: (vars) => `/api/x/things/${vars.id}`,
      });
      return (
        <button type="button" onClick={() => mutation.mutate({ id: 9 })}>
          delete
        </button>
      );
    }
    withClient(<Probe />);
    const button = await screen.findByRole("button");
    act(() => button.click());
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe("/api/x/things/9");
  });
});

describe("usePluginStream", () => {
  function sseStream(frames: string[]): ReadableStream<Uint8Array> {
    const encoder = new TextEncoder();
    return new ReadableStream({
      start(controller) {
        for (const frame of frames) controller.enqueue(encoder.encode(frame));
        controller.close();
      },
    });
  }

  it("delivers each SSE frame and reports done", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        sseStream([
          'event: step\ndata: {"n":1}\n\n',
          ": heartbeat\n\n",
          'data: {"n":2}\n\n',
        ]),
        { status: 200, headers: { "content-type": "text/event-stream" } },
      ),
    );
    const seen: string[] = [];
    let done = false;
    function Probe() {
      const [count, setCount] = useState(0);
      usePluginStream<{ n: number }>({
        path: "/api/x/stream",
        body: { q: 1 },
        onEvent: (e) => {
          seen.push(`${e.event}:${e.data.n}`);
          setCount((c) => c + 1);
        },
        onDone: () => {
          done = true;
        },
      });
      return <div data-testid="count">{count}</div>;
    }
    withClient(<Probe />);
    await waitFor(() => expect(seen).toEqual(["step:1", "message:2"]));
    await waitFor(() => expect(done).toBe(true));
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect((init.headers as Record<string, string>).Accept).toBe("text/event-stream");
    expect(JSON.parse(String(init.body))).toEqual({ q: 1 });
  });

  it("surfaces a refused stream as an ApiError", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ message: "no budget" }, { status: 402 }));
    let failure: ApiError | null = null;
    function Probe() {
      usePluginStream({
        path: "/api/x/stream",
        onEvent: () => {},
        onError: (e) => {
          failure = e;
        },
      });
      return <div />;
    }
    withClient(<Probe />);
    await waitFor(() => expect(failure?.status).toBe(402));
    expect(failure!.message).toBe("no budget");
  });

  it("aborts the request on unmount", async () => {
    let signal: AbortSignal | undefined;
    fetchMock.mockImplementationOnce(async (_url: string, init: RequestInit) => {
      signal = init.signal ?? undefined;
      return await new Promise<Response>((_resolve, reject) => {
        init.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
      });
    });
    function Probe() {
      usePluginStream({ path: "/api/x/stream", onEvent: () => {} });
      return <div />;
    }
    const tree = withClient(<Probe />);
    await waitFor(() => expect(signal).toBeDefined());
    tree.unmount();
    expect(signal!.aborted).toBe(true);
  });
});
