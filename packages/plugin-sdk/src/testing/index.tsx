// @octarq/plugin-sdk/testing — the test harness a plugin's own test suite
// mounts its pages in.
//
// A plugin page reads four contexts (react-query, i18n, brand, plugin gate) and
// talks to the backend through fetch. Mounting a page by hand in every test
// file drifts fast. This subpath supplies both halves: `TestHarness` provides
// the contexts with sane defaults, and `mockPluginApi` answers the routes a
// test declares and records the calls.
//
// Nothing here imports a test runner: it works under vitest, jest, or a plain
// node:test run.
import { useState, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider, type Resources } from "../i18n";
import { BrandProvider } from "../brand";
import { PluginGateContext, type PluginGateContextValue } from "../contract/PluginGateContext";
import { resetPluginHttp } from "../data/request";

export interface TestHarnessProps {
  children: ReactNode;
  // i18n resources; omit to render English keys as their fallbacks.
  resources?: Resources;
  // Product name the brand hook returns. Default "octarq".
  brand?: string;
  // Plugin gate state. Default: everything enabled and loaded.
  gate?: Partial<PluginGateContextValue>;
  // Share one client across assertions when a test inspects the cache.
  queryClient?: QueryClient;
  // App-specific providers (router, theme, …) the host would mount.
  wrapper?: (children: ReactNode) => ReactNode;
}

// TestHarness mounts a plugin page under the same providers the host app does.
export function TestHarness({
  children,
  resources,
  brand = "octarq",
  gate,
  queryClient,
  wrapper,
}: TestHarnessProps) {
  // One client per mount, retries off: a failing request fails the test now
  // instead of three backoffs later. The initializer also clears any base path
  // a previous test configured — it runs before the children mount, so no
  // request of this test can see the stale value.
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
  // HTTP method; defaults to GET.
  method?: string;
  // Exact path or pattern to match against the request URL.
  path: string | RegExp;
  status?: number;
  // Parsed JSON body returned with status 200 (or the given status).
  body?: unknown;
  headers?: Record<string, string>;
}

export interface MockCall {
  method: string;
  url: string;
  body?: unknown;
}

export interface MockPluginApi {
  // Installable as globalThis.fetch.
  fetch: typeof fetch;
  // Every request the code made, in order.
  calls: MockCall[];
  // Puts the previous global fetch back.
  restore: () => void;
}

function matches(route: MockRoute, method: string, url: string): boolean {
  if ((route.method ?? "GET").toUpperCase() !== method) return false;
  return typeof route.path === "string" ? route.path === url : route.path.test(url);
}

// mockPluginApi installs a fetch that answers the declared routes and records
// each call. An undeclared route answers 404 with a JSON body, so a typo shows
// up as a failing assertion instead of a hanging request.
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
