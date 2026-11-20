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
  path: string | ((vars: TVars) => string);
  body?: (vars: TVars) => unknown;
  invalidate?: readonly (readonly unknown[])[] | ((data: TData, vars: TVars) => readonly (readonly unknown[])[]);
  onSuccess?: (data: TData, vars: TVars) => void;
  onError?: (error: ApiError, vars: TVars) => void;
}

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
  event: string;
  data: T;
}

export interface PluginStreamOptions<T> {
  path: string;
  method?: string;
  body?: unknown;
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
    return { event, data: raw as unknown as T };
  }
}

export function usePluginStream<T = unknown>(options: PluginStreamOptions<T>): PluginStreamHandle {
  const [running, setRunning] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
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
            if (frame.startsWith(":")) continue;
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
  }, [enabled, path, method, bodyKey, start]);

  return { start, stop, running };
}
