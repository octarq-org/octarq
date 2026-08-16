// Global network guard for the vitest suite.
//
// A test can silently fall through to REAL network when a mock runs dry: a
// spy stacked with mockResolvedValueOnce responses calls the real
// implementation once its queue is consumed, a module fetches at import time,
// a stub is forgotten. The failure mode that follows is machine-dependent —
// locally the request may hang on a proxy, in CI it may be refused and some
// other branch "happens to pass". The worst part is the run looks green while
// it is touching the network at all.
//
// This file replaces globalThis.fetch with a stub that turns every real
// request into an immediate, deterministic rejection naming the method + URL
// and pointing at the missing mock. It is installed for EVERY test file via
// vitest.config.ts `setupFiles`, so no test can make a live request by
// accident — and if one tries, the error says which mock was not covered.
//
// Deliberate opt-out (only for tests that genuinely need the network):
//   import { allowRealNetwork } from "./test/setup";
//   allowRealNetwork();

// Set to false only to verify the guard itself fails (see networkGuard.test.ts).
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

// Rejects rather than throwing synchronously so both `await fetch(...)` call
// sites and `expect(fetch(...)).rejects...` assertions observe the same error.
globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
  if (allowNetwork) return realFetch(input, init);
  const method = (init?.method ?? "GET").toUpperCase();
  const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
  if (BLOCK) return blocked(method, url);
  return realFetch(input, init);
}) as typeof fetch;
