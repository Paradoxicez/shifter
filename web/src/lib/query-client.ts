import { QueryClient } from '@tanstack/react-query'

/**
 * Shared TanStack Query client for the SPA.
 *
 * Defaults:
 * - retry: 1 (one extra attempt on transient failures, then surface)
 * - refetchOnWindowFocus: false (operator console UX — don't refetch when
 *   the operator briefly tabs away)
 * - staleTime: 30s (most settings/identity data is read-mostly)
 * - mutations.retry: 0 (do not auto-retry POST/PUT/DELETE)
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30 * 1000,
    },
    mutations: { retry: 0 },
  },
})
