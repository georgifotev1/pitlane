import path from "path"
import babel from "@rolldown/plugin-babel"
import tailwindcss from "@tailwindcss/vite"
import lingui, { linguiTransformerBabelPreset } from "@lingui/vite-plugin"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
    lingui(),
    // The Lingui macros (`@lingui/react/macro`, `@lingui/core/macro`) are
    // compile-time transforms. @vitejs/plugin-react v6 uses oxc (no babel
    // hook), so the macro transform runs as a separate babel pass; without it
    // the macros throw "executed outside the context of compilation" at
    // runtime.
    babel({ presets: [linguiTransformerBabelPreset()] }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:4000",
    },
  },
})
