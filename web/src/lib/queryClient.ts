import { QueryClient } from "@tanstack/react-query";

/**
 * Shared TanStack QueryClient instance for managing server state across Octarq Web.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});
