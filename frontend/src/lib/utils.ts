import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Returns true when a click landed on a real control *inside* a clickable
// container (a button, link, form field, etc.) so the container's own click
// handler can bail and let the control handle it. `container` is excluded from
// the search: clickable rows carry role="link" themselves, and closest() would
// otherwise match the row for every click and swallow it.
export function isInteractiveTarget(
  target: EventTarget | null,
  container: HTMLElement,
): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const el = target.closest(
    "button, a, input, textarea, select, [role='button'], [role='link']",
  );
  return !!el && el !== container && container.contains(el);
}

export function formatMoney(cents: number): string {
  return (cents / 100).toLocaleString("bg-BG", {
    style: "currency",
    currency: "EUR",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
