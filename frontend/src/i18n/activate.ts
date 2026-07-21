import { i18n } from "@lingui/core"
import { detectLocale, persistLocale, type Locale } from "./detector"

const catalogs: Record<Locale, () => Promise<{ messages: Record<string, string> }>> = {
  bg: async () => (await import("@/locales/bg")).default,
}

let activated = false

export async function activateLocale(locale: Locale = detectLocale()): Promise<void> {
  if (activated && i18n.locale === locale) return
  const load = catalogs[locale]
  const { messages } = await load()
  i18n.load(locale, messages)
  i18n.activate(locale)
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale
  }
  activated = true
}

export async function setLocale(locale: Locale): Promise<void> {
  await activateLocale(locale)
  persistLocale(locale)
}
