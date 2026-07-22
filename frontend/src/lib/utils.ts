import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// formatMoney renders integer cents as a euro amount in the bg-BG locale
// ("150,00 €"). Bulgaria adopted the euro on 2026-01-01 and the app is
// euro-denominated, so the currency is fixed here rather than plumbed from the
// tenant — matching the "€" the server prints on the PDF and offer email.
export function formatMoney(cents: number): string {
  return (cents / 100).toLocaleString("bg-BG", {
    style: "currency",
    currency: "EUR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}
