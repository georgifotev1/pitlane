// Locale detection order: explicit localStorage choice, then the browser
// language, then a hard default of Bulgarian. There is no EN fallback; the
// project ships BG-only for now (ADR §29 / Phase 2.5).
const STORAGE_KEY = "pitlane:locale"
export const DEFAULT_LOCALE = "bg" as const
export type Locale = typeof DEFAULT_LOCALE
export const SUPPORTED_LOCALES: ReadonlyArray<Locale> = [DEFAULT_LOCALE]

function isSupported(value: string | null): value is Locale {
  return value !== null && (SUPPORTED_LOCALES as ReadonlyArray<string>).includes(value)
}

export function detectLocale(): Locale {
  if (typeof window === "undefined") return DEFAULT_LOCALE
  const stored = window.localStorage.getItem(STORAGE_KEY)
  if (isSupported(stored)) return stored
  const nav = window.navigator.language.toLowerCase()
  if (isSupported(nav)) return nav
  // Short-prefix match (e.g. "bg-BG" -> "bg").
  const short = nav.split("-")[0]
  if (short !== undefined && isSupported(short)) return short
  return DEFAULT_LOCALE
}

export function persistLocale(locale: Locale): void {
  if (typeof window === "undefined") return
  window.localStorage.setItem(STORAGE_KEY, locale)
}
