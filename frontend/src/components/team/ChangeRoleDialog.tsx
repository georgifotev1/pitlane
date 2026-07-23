import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import type { UserResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { applyServerErrors } from "@/lib/formErrors"
import { queryKeys } from "@/lib/queryKeys"
import { ASSIGNABLE_ROLES, useRoleLabel } from "@/lib/roles"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: UserResponse
}

type RoleForm = {
  role: string
}

/**
 * Changes a teammate's role. The owner's role and one's own role are not
 * changeable — the caller only opens this for eligible users, and the server
 * backstops both with 409s (cannot_change_owner_role / cannot_change_own_role),
 * surfaced here as a root error.
 */
export function ChangeRoleDialog({ open, onOpenChange, user }: Props) {
  const qc = useQueryClient()
  const roleLabel = useRoleLabel()
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<RoleForm>({ values: { role: user.role } })

  const mutation = useMutation({
    mutationFn: (v: RoleForm) => api.users.updateRole(user.id, v),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.users.list() })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Change role</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Set the permissions for {user.name}.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <Label htmlFor="role">
              <Trans>Role</Trans>
            </Label>
            <select
              id="role"
              className="h-9 w-full rounded-lg border border-input bg-background px-3 text-sm"
              {...register("role")}
            >
              {ASSIGNABLE_ROLES.map((r) => (
                <option key={r} value={r}>
                  {roleLabel(r)}
                </option>
              ))}
            </select>
          </div>
          {errors.root && <p className="text-sm text-destructive">{errors.root.message}</p>}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting || mutation.isPending}
            >
              <Trans>Cancel</Trans>
            </Button>
            <Button type="submit" disabled={isSubmitting || mutation.isPending}>
              {mutation.isPending ? <Trans>Saving…</Trans> : <Trans>Save</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
