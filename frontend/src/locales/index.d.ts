// Lingui compiles each catalog to an ES module that exports `messages` as a
// named export (`compileNamespace: "es"` in `lingui.config.ts`). ESM output is
// required because Vite loads the catalog directly in the browser, where the
// CommonJS `module.exports` form throws "module is not defined". The path
// matches the locale in `lingui.config.ts`; the compiled artifact is at
// `src/locales/<locale>.mjs`.
declare module "@/locales/bg" {
  export const messages: Record<string, string>
}
