/**
 * Centralized TanStack Query keys. Using a factory keeps keys unique and
 * greppable, and makes invalidation explicit (queryKeys.auth.me() invalidates
 * the "me" query regardless of which component reads it).
 */
import type {
  CarBoardQuery,
  CarListQuery,
  CustomerListQuery,
  OfferBoardQuery,
  OfferListQuery,
  RepairListQuery,
} from "@/lib/api";

export const queryKeys = {
  health: () => ["health"] as const,
  auth: {
    me: () => ["auth", "me"] as const,
  },
  users: {
    all: () => ["users"] as const,
    list: () => ["users", "list"] as const,
    invitations: () => ["users", "invitations"] as const,
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
    board: (q: CarBoardQuery) => ["cars", "board", q] as const,
    detail: (id: string) => ["cars", "detail", id] as const,
  },
  offers: {
    all: () => ["offers"] as const,
    listForCar: (carId: string, q: OfferListQuery) =>
      ["offers", "list", carId, q] as const,
    board: (q: OfferBoardQuery) => ["offers", "board", q] as const,
    detail: (id: string) => ["offers", "detail", id] as const,
  },
  repairs: {
    all: () => ["repairs"] as const,
    list: (q: RepairListQuery) => ["repairs", "list", q] as const,
    detail: (id: string) => ["repairs", "detail", id] as const,
    attachments: (repairId: string) => ["repairs", "attachments", repairId] as const,
  },
  history: {
    all: () => ["history"] as const,
    forCar: (carId: string) => ["history", "car", carId] as const,
  },
  attachments: {
    all: () => ["attachments"] as const,
    forCar: (carId: string) => ["attachments", "car", carId] as const,
    forRepair: (repairId: string) => ["attachments", "repair", repairId] as const,
  },
} as const;
