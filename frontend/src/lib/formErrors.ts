import type { UseFormSetError, FieldValues, Path } from "react-hook-form"
import { ProblemError } from "@/lib/api"
import { errorCodeToMessage } from "@/lib/errorCodes"

/**
 * Applies a 422 problem+json's `errors` map to a React Hook Form via setError.
 * Each value in the map is a server-emitted code; we resolve it to a localized
 * message via `errorCodeToMessage`. Fields not present in the map are left
 * untouched; a top-level error (`code` on the problem itself) surfaces as a
 * root form error under the key "root".
 *
 * `mapField` optionally rewrites a server field key onto a form field name when
 * the two diverge (e.g. the offer editor holds `unitPrice` while the server
 * validates `unitPriceCents`).
 */
export function applyServerErrors<T extends FieldValues>(
  setError: UseFormSetError<T>,
  err: unknown,
  mapField?: (field: string) => string,
): void {
  if (!(err instanceof ProblemError)) return

  if (err.errors) {
    for (const [field, code] of Object.entries(err.errors)) {
      const mapped = mapField ? mapField(field) : field
      const key = (mapped === "_form" || mapped === "form" ? "root" : mapped) as Path<T>
      setError(key, { type: "server", message: errorCodeToMessage(code) })
    }
    return
  }

  if (err.code) {
    setError("root" as Path<T>, { type: "server", message: errorCodeToMessage(err.code) })
  }
}
