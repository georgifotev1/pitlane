/**
 * Centralized TanStack Query keys. Using a factory keeps keys unique and
 * greppable, and makes invalidation explicit (queryKeys.auth.me() invalidates
 * the "me" query regardless of which component reads it).
 */
export const queryKeys = {
  health: () => ["health"] as const,
  auth: {
    me: () => ["auth", "me"] as const,
  },
} as const;
