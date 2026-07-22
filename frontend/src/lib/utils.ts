import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// formatMoney renders integer cents as a 2-decimal amount in the bg-BG locale
// (no currency symbol — the tenant currency is not yet exposed to the client).
export function formatMoney(cents: number): string {
  return (cents / 100).toLocaleString("bg-BG", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}
