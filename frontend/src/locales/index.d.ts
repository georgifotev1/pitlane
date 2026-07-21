// Lingui compiles each catalog to a CommonJS-style JS module that exports
// `{ messages }` as the default export. The path matches the locale in
// `lingui.config.ts` and `runtimeConfigModule: ["@lingui/core", "i18n"]`.
// The compiled artifact is at `src/locales/<locale>.js`.
declare module "@/locales/bg" {
  const catalog: { messages: Record<string, string> }
  export default catalog
}
