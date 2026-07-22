/**
 * Centralized TanStack Query keys. Using a factory keeps keys unique and
 * greppable, and makes invalidation explicit (queryKeys.auth.me() invalidates
 * the "me" query regardless of which component reads it).
 */
import type { CarListQuery, CustomerListQuery, OfferListQuery } from "@/lib/api";

export const queryKeys = {
  health: () => ["health"] as const,
  auth: {
    me: () => ["auth", "me"] as const,
  },
  customers: {
    all: () => ["customers"] as const,
    list: (q: CustomerListQuery) => ["customers", "list", q] as const,
    detail: (id: string) => ["customers", "detail", id] as const,
  },
  cars: {
    all: () => ["cars"] as const,
    listForCustomer: (customerId: string, q: CarListQuery) =>
      ["cars", "list", customerId, q] as const,
    detail: (id: string) => ["cars", "detail", id] as const,
  },
  offers: {
    all: () => ["offers"] as const,
    listForCar: (carId: string, q: OfferListQuery) =>
      ["offers", "list", carId, q] as const,
    detail: (id: string) => ["offers", "detail", id] as const,
  },
} as const;
