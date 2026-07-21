import { defineConfig } from "@lingui/conf"
import { formatter } from "@lingui/format-po"

export default defineConfig({
  sourceLocale: "bg",
  locales: ["bg"],
  fallbackLocales: false,
  catalogs: [
    {
      path: "src/locales/{locale}",
      include: ["src"],
      exclude: ["**/dist/**", "**/node_modules/**"],
    },
  ],
  format: formatter({ lineNumbers: false }),
  runtimeConfigModule: ["@lingui/core", "i18n"],
})
