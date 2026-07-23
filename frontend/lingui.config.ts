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
  // Vite loads the compiled catalog as a browser ES module, so emit ESM
  // (`export`) rather than the default CommonJS (`module.exports`), which
  // throws "module is not defined" at runtime.
  compileNamespace: "es",
  runtimeConfigModule: ["@lingui/core", "i18n"],
})
