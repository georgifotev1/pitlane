import { I18nProvider as LinguiProvider } from "@lingui/react"
import { type ReactNode, useEffect, useState } from "react"
import { activateLocale } from "./activate"
import { i18n } from "./config"

export function I18nProvider({ children }: { children: ReactNode }) {
  const [ready, setReady] = useState(false)

  useEffect(() => {
    let cancelled = false
    activateLocale().then(() => {
      if (!cancelled) setReady(true)
    })
    return () => {
      cancelled = true
    }
  }, [])

  if (!ready) return null
  return <LinguiProvider i18n={i18n}>{children}</LinguiProvider>
}
