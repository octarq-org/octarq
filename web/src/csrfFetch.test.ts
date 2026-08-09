/**
 * @vitest-environment happy-dom
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';

const mockFetch = vi.fn().mockResolvedValue(new Response());
window.fetch = mockFetch as any;

// Ensure wrapper runs
await import('./csrfFetch');

describe('csrfFetch', () => {
  beforeEach(() => {
    mockFetch.mockClear();
    document.cookie = '';
    Object.defineProperty(window, 'location', {
      value: { origin: 'http://localhost:3000', href: 'http://localhost:3000/' },
      writable: true
    });
  });

  it('injects header for same-origin write request', async () => {
    document.cookie = 'octarq_csrf=my-token; path=/';
    await window.fetch('http://localhost:3000/api', { method: 'POST' });
    
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const args = mockFetch.mock.calls[0];
    const init = args[1];
    const headers = new Headers(init.headers);
    expect(headers.get('X-CSRF-Token')).toBe('my-token');
  });

  it('does not inject for same-origin GET', async () => {
    document.cookie = 'octarq_csrf=my-token; path=/';
    await window.fetch('http://localhost:3000/api', { method: 'GET' });
    
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const args = mockFetch.mock.calls[0];
    const init = args[1];
    if (init?.headers) {
      const headers = new Headers(init.headers);
      expect(headers.has('X-CSRF-Token')).toBe(false);
    } else {
      expect(init?.headers).toBeUndefined();
    }
  });

  it('does not inject for cross-origin write request', async () => {
    document.cookie = 'octarq_csrf=my-token; path=/';
    await window.fetch('https://evil.com/api', { method: 'POST' });
    
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const args = mockFetch.mock.calls[0];
    const init = args[1];
    if (init?.headers) {
      const headers = new Headers(init.headers);
      expect(headers.has('X-CSRF-Token')).toBe(false);
    } else {
      expect(init?.headers).toBeUndefined();
    }
  });

  it('does not overwrite existing header', async () => {
    document.cookie = 'octarq_csrf=my-token; path=/';
    await window.fetch('http://localhost:3000/api', { 
      method: 'POST', 
      headers: { 'X-CSRF-Token': 'existing-token' } 
    });
    
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const args = mockFetch.mock.calls[0];
    const init = args[1];
    const headers = new Headers(init.headers);
    expect(headers.get('X-CSRF-Token')).toBe('existing-token');
  });

  it('handles relative URLs correctly (same-origin)', async () => {
    document.cookie = 'octarq_csrf=my-token; path=/';
    await window.fetch('/api/v1/test', { method: 'PUT' });
    
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const args = mockFetch.mock.calls[0];
    const init = args[1];
    const headers = new Headers(init.headers);
    expect(headers.get('X-CSRF-Token')).toBe('my-token');
  });
});
