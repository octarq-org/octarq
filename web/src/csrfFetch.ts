const originalFetch = window.fetch;

function getCsrfToken(): string | null {
  const match = document.cookie.match(/(?:^|;\s*)octarq_csrf=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : null;
}

function isWriteMethod(method?: string): boolean {
  if (!method) return false;
  const m = method.toUpperCase();
  return m === 'POST' || m === 'PUT' || m === 'PATCH' || m === 'DELETE';
}

function isSameOrigin(target: string): boolean {
  try {
    const url = new URL(target, window.location.origin);
    return url.origin === window.location.origin;
  } catch {
    return false;
  }
}

window.fetch = async function(input: RequestInfo | URL, init?: RequestInit) {
  let url = '';
  let method = 'GET';
  let hasHeader = false;

  if (input instanceof Request) {
    url = input.url;
    method = init?.method || input.method || 'GET';
    hasHeader = input.headers.has('X-CSRF-Token');
  } else if (input instanceof URL) {
    url = input.href;
    method = init?.method || 'GET';
  } else {
    url = input;
    method = init?.method || 'GET';
  }

  if (init?.headers) {
    hasHeader = hasHeader || new Headers(init.headers).has('X-CSRF-Token');
  }

  if (isWriteMethod(method) && isSameOrigin(url) && !hasHeader) {
    const token = getCsrfToken();
    if (token) {
      if (input instanceof Request) {
        const newHeaders = new Headers(input.headers);
        if (init?.headers) {
          const initHeaders = new Headers(init.headers);
          initHeaders.forEach((v, k) => newHeaders.set(k, v));
        }
        newHeaders.set('X-CSRF-Token', token);
        init = { ...init, headers: newHeaders };
      } else {
        const newHeaders = new Headers(init?.headers);
        newHeaders.set('X-CSRF-Token', token);
        init = { ...init, headers: newHeaders };
      }
    }
  }

  return originalFetch.call(this, input, init);
};

export {};
