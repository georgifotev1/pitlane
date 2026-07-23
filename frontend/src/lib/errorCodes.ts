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
  duplicate: "Този регистрационен номер вече съществува.",
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
  conflict: "Действието е в конфликт с текущото състояние (офертата вече е изпратена).",
  rate_limit_exceeded: "Твърде много заявки. Изчакайте и опитайте отново.",
  // Phase 10: password reset + staff invitations.
  invalid_token: "Връзката е невалидна или е изтекла. Поискайте нова.",
  email_taken: "Този имейл вече е зает.",
  already_invited: "Вече има изпратена покана за този имейл.",
  cannot_change_own_role: "Не можете да променяте собствената си роля.",
  cannot_change_owner_role: "Ролята на собственика не може да бъде променяна.",
}

export function errorCodeToMessage(code: string | undefined | null): string {
  if (!code) return ""
  return messages[code] ?? code
}
