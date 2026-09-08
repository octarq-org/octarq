import { QueryClient } from "@tanstack/react-query";

/**
 * Shared TanStack QueryClient instance for managing server state across Octarq Web.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 30,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});
