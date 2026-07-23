import { useLingui } from "@lingui/react/macro"

// Roles are stable server codes ("owner" | "admin" | "mechanic"); their
// Bulgarian labels are UI concerns. useRoleLabel returns a resolver bound to the
// active catalog so tables and selects render one consistent label.
export function useRoleLabel(): (role: string) => string {
  const { t } = useLingui()
  return (role: string) => {
    switch (role) {
      case "owner":
        return t`Owner`
      case "admin":
        return t`Administrator`
      case "mechanic":
        return t`Mechanic`
      default:
        return role
    }
  }
}

// Only staff roles are invitable / assignable through the team screen — the
// owner is born at signup and its role is immutable (server enforces both).
export const ASSIGNABLE_ROLES = ["admin", "mechanic"] as const
