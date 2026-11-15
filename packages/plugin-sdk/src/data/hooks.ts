// Query and stream hooks over the plugin HTTP surface.
//
// The host app owns the single @tanstack/react-query provider; these hooks
// consume that context, so a plugin page shares the host's cache, retries and
// invalidation instead of shipping a second QueryClient.
import { useCallback, useEffect, useRef, useState } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { ApiError, getAppLang, pluginURL, req } from "./request";

export interface PluginDataOptions {
  enabled?: boolean;
  staleTime?: number;
  refetchInterval?: number | false;
}

// usePluginData reads one GET route. `key` is the cache key, so two pages that
// ask for the same resource share one request; `path` is the route.
export function usePluginData<T>(
  key: readonly unknown[],
  path: string,
  options: PluginDataOptions = {},
): UseQueryResult<T, ApiError> {
  return useQuery<T, ApiError>({
    queryKey: key,
    queryFn: ({ signal }) => req<T>("GET", path, undefined, undefined, signal),
    enabled: options.enabled,
    staleTime: options.staleTime,
    refetchInterval: options.refetchInterval,
  });
}

export interface PluginMutationOptions<TVars, TData> {
  method?: string;
  // A fixed route, or one derived from the variables (e.g. an item id).
  path: string | ((vars: TVars) => string);
  // Builds the JSON body from the variables. Omit for a bodyless call.
  body?: (vars: TVars) => unknown;
  // Cache keys to invalidate on success. A function form can narrow the keys
  // to what the mutation touched.
  invalidate?: readonly (readonly unknown[])[] | ((data: TData, vars: TVars) => readonly (readonly unknown[])[]);
  onSuccess?: (data: TData, vars: TVars) => void;
  onError?: (error: ApiError, vars: TVars) => void;
}

// usePluginMutation writes one route and invalidates the listed cache keys.
// The error keeps its status, so a caller can branch on 402/403/404.
export function usePluginMutation<TVars = void, TData = unknown>(
  options: PluginMutationOptions<TVars, TData>,
): UseMutationResult<TData, ApiError, TVars> {
  const client = useQueryClient();
  const method = options.method ?? "POST";
  return useMutation<TData, ApiError, TVars>({
    mutationFn: (vars) => {
      const path = typeof options.path === "function" ? options.path(vars) : options.path;
      return req<TData>(method, path, options.body ? options.body(vars) : undefined);
    },
    onSuccess: (data, vars) => {
      const keys =
        typeof options.invalidate === "function" ? options.invalidate(data, vars) : options.invalidate;
      for (const key of keys ?? []) void client.invalidateQueries({ queryKey: key });
      options.onSuccess?.(data, vars);
    },
    onError: (error, vars) => options.onError?.(error, vars),
  });
}

export interface PluginStreamEvent<T> {
  // The SSE `event:` name; "message" when the frame names none.
  event: string;
  data: T;
}

export interface PluginStreamOptions<T> {
  // The route to subscribe to. A POST route carries `body`; a GET route omits it.
  path: string;
  method?: string;
  body?: unknown;
  // Starts the stream automatically. Default true.
  enabled?: boolean;
  onEvent: (event: PluginStreamEvent<T>) => void;
  onError?: (error: ApiError) => void;
  onDone?: () => void;
}

export interface PluginStreamHandle {
  start: () => void;
  stop: () => void;
  running: boolean;
}

// splitFrames cuts every complete SSE frame ("\n\n"-terminated) out of the
// buffer and returns the unfinished tail.
function splitFrames(buffer: string): { frames: string[]; rest: string } {
  const frames: string[] = [];
  let rest = buffer;
  let at: number;
  while ((at = rest.indexOf("\n\n")) !== -1) {
    frames.push(rest.slice(0, at));
    rest = rest.slice(at + 2);
  }
  return { frames, rest };
}

function parseFrame<T>(frame: string): PluginStreamEvent<T> | null {
  let event = "message";
  const data: string[] = [];
  for (const line of frame.split("\n")) {
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  if (data.length === 0) return null;
  const raw = data.join("\n");
  try {
    return { event, data: JSON.parse(raw) as T };
  } catch {
    // A non-JSON payload is delivered as text rather than dropped.
    return { event, data: raw as unknown as T };
  }
}

// usePluginStream subscribes to one SSE route. It aborts on unmount and on a
// path/body change, so a component that navigates away leaves no open
// connection behind.
export function usePluginStream<T = unknown>(options: PluginStreamOptions<T>): PluginStreamHandle {
  const [running, setRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  // Callbacks live in a ref: an inline handler on every render must not
  // resubscribe the stream.
  const handlers = useRef(options);
  handlers.current = options;

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setRunning(false);
  }, []);

  const start = useCallback(() => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setRunning(true);

    const opts = handlers.current;
    void (async () => {
      try {
        const headers: Record<string, string> = {
          Accept: "text/event-stream",
          "Accept-Language": getAppLang(),
        };
        if (opts.body !== undefined) headers["Content-Type"] = "application/json";
        const res = await fetch(pluginURL(opts.path), {
          method: opts.method ?? "POST",
          headers,
          body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
          credentials: "same-origin",
          signal: controller.signal,
        });
        if (!res.ok) {
          let parsed: unknown = null;
          try {
            parsed = await res.json();
          } catch {
            /* not JSON */
          }
          const body = parsed as { message?: string } | null;
          throw new ApiError(
            res.status,
            body?.message ?? res.statusText ?? `HTTP ${res.status}`,
            parsed,
            res.headers.get("x-request-id") ?? undefined,
          );
        }
        if (!res.body) throw new ApiError(502, "The response carried no stream.");
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const { frames, rest } = splitFrames(buffer);
          buffer = rest;
          for (const frame of frames) {
            if (frame.startsWith(":")) continue; // heartbeat comment
            const parsed = parseFrame<T>(frame);
            if (parsed) handlers.current.onEvent(parsed);
          }
        }
        if (controller.signal.aborted) return;
        setRunning(false);
        handlers.current.onDone?.();
      } catch (error) {
        if (controller.signal.aborted) return;
        setRunning(false);
        const apiError =
          error instanceof ApiError ? error : new ApiError(0, error instanceof Error ? error.message : String(error));
        handlers.current.onError?.(apiError);
      }
    })();
  }, []);

  const { path, method, body, enabled = true } = options;
  const bodyKey = body === undefined ? "" : JSON.stringify(body);
  useEffect(() => {
    if (!enabled) return undefined;
    start();
    return () => {
      abortRef.current?.abort();
      abortRef.current = null;
    };
    // path/method/bodyKey identify the subscription; callback and enabled are
    // read through `handlers`.
  }, [enabled, path, method, bodyKey, start]);

  return { start, stop, running };
}
