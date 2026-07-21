// `i18n` is the Lingui runtime config module referenced from `lingui.config.ts`
// (runtimeConfigModule). The compile step wires the build-time catalog into
// `loadAndActivate` so we never ship a runtime fetch for translations.
import { i18n } from "@lingui/core"

export { i18n }
