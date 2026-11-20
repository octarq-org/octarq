
const BLOCK = true;

const realFetch = globalThis.fetch;

let allowNetwork = false;

export function allowRealNetwork(): void {
  allowNetwork = true;
}

function blocked(method: string, url: string): Promise<never> {
  return Promise.reject(
    new Error(
      `Network request blocked in tests: ${method} ${url}. ` +
        "A mock did not cover this call — stub it explicitly, or call allowRealNetwork() " +
        "before the request if real network is intentional.",
    ),
  );
}

globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
  if (allowNetwork) return realFetch(input, init);
  const method = (init?.method ?? "GET").toUpperCase();
  const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
  if (BLOCK) return blocked(method, url);
  return realFetch(input, init);
}) as typeof fetch;
