// Maps server-emitted problem+json error codes to localized (BG) messages.
// Codes are the stable contract; the server never sends user-facing strings.
// The fallback (`code`) is shown if a new server code hasn't been added here
// yet — that's an early warning during development, not a UX failure.
//
// The server side of this contract lives in:
//   api/internal/api/render.go (CodeXxx constants)
//   api/internal/validator/validator.go (CodeXxx constants)
const messages: Record<string, string> = {
  // 422 field codes
  required: "Полето е задължително.",
  invalid_email: "Невалиден имейл адрес.",
  too_short: "Стойността е твърде кратка.",
  too_long: "Стойността е твърде дълга.",
  invalid: "Невалидна стойност.",
  // 422 top-level
  validation_failed: "Моля, проверете въведените данни.",
  // global codes
  invalid_credentials: "Невалиден имейл или парола.",
  invalid_json: "Невалидно тяло на заявката.",
  internal_error: "Вътрешна грешка. Опитайте отново по-късно.",
  account_creation_failed: "Акаунтът не може да бъде създаден. Опитайте отново.",
  authentication_required: "Необходимо е влизане в профила.",
  permission_denied: "Нямате право за това действие.",
  not_found: "Ресурсът не е намерен.",
  rate_limit_exceeded: "Твърде много заявки. Изчакайте и опитайте отново.",
}

export function errorCodeToMessage(code: string | undefined | null): string {
  if (!code) return ""
  return messages[code] ?? code
}
