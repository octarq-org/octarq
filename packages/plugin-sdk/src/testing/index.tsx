import { useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider, type Resources } from "../i18n";
import { BrandProvider } from "../brand";
import { PluginGateContext, type PluginGateContextValue } from "../contract/PluginGateContext";
import { resetPluginHttp } from "../data/request";

export interface TestHarnessProps {
  children: ReactNode;
  resources?: Resources;
  brand?: string;
  gate?: Partial<PluginGateContextValue>;
  queryClient?: QueryClient;
  wrapper?: (children: ReactNode) => ReactNode;
}

export function TestHarness({
  children,
  resources,
  brand = "octarq",
  gate,
  queryClient,
  wrapper,
}: TestHarnessProps) {
  const [client] = useState(() => {
    resetPluginHttp();
    return (
      queryClient ??
      new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
    );
  });

  const gateValue: PluginGateContextValue = {
    disabledPlugins: new Set(),
    disabledPaths: new Set(),
    loaded: true,
    ...gate,
  };

  let tree = (
    <QueryClientProvider client={client}>
      <I18nProvider resources={resources}>
        <BrandProvider name={brand}>
          <PluginGateContext.Provider value={gateValue}>{children}</PluginGateContext.Provider>
        </BrandProvider>
      </I18nProvider>
    </QueryClientProvider>
  );
  if (wrapper) tree = <>{wrapper(tree)}</>;
  return tree;
}

export interface MockRoute {
  method?: string;
  path: string | RegExp;
  status?: number;
  body?: unknown;
  headers?: Record<string, string>;
}

export interface MockCall {
  method: string;
  url: string;
  body?: unknown;
}

export interface MockPluginApi {
  fetch: typeof fetch;
  calls: MockCall[];
  restore: () => void;
}

function matches(route: MockRoute, method: string, url: string): boolean {
  if ((route.method ?? "GET").toUpperCase() !== method) return false;
  return typeof route.path === "string" ? route.path === url : route.path.test(url);
}

export function mockPluginApi(routes: MockRoute[]): MockPluginApi {
  const calls: MockCall[] = [];
  const original = globalThis.fetch;
  const impl: typeof fetch = async (input, init) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    const method = (init?.method ?? "GET").toUpperCase();
    let body: unknown;
    if (typeof init?.body === "string" && init.body) {
      try {
        body = JSON.parse(init.body);
      } catch {
        body = init.body;
      }
    }
    calls.push({ method, url, body });

    const route = routes.find((r) => matches(r, method, url));
    if (!route) {
      return new Response(JSON.stringify({ message: `No mock route for ${method} ${url}` }), {
        status: 404,
        headers: { "content-type": "application/json" },
      });
    }
    const status = route.status ?? 200;
    const headers: Record<string, string> = { "content-type": "application/json", ...route.headers };
    if (status === 204) return new Response(null, { status, headers });
    return new Response(JSON.stringify(route.body ?? {}), { status, headers });
  };
  globalThis.fetch = impl;
  return {
    fetch: impl,
    calls,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}
