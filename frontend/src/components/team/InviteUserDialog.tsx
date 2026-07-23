import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type InviteForm = {
  email: string
  role: string
}

/**
 * Invites a teammate by email + role. The invitee completes their account from
 * the emailed link (/accept-invite). Server 422s (email_taken, already_invited)
 * land on the email field via applyServerErrors.
 */
export function InviteUserDialog({ open, onOpenChange }: Props) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const roleLabel = useRoleLabel()
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<InviteForm>({ defaultValues: { email: "", role: "mechanic" } })

  const mutation = useMutation({
    mutationFn: (v: InviteForm) => api.users.invitations.create(v),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.users.invitations() })
      reset({ email: "", role: "mechanic" })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Invite teammate</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>They'll get an email with a link to set up their account.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <Label htmlFor="email">
              <Trans>Email</Trans>
            </Label>
            <Input
              id="email"
              type="email"
              autoComplete="off"
              {...register("email", { required: t`This field is required.` })}
            />
            {errors.email && <p className="text-sm text-destructive">{errors.email.message}</p>}
          </div>
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
            {errors.role && <p className="text-sm text-destructive">{errors.role.message}</p>}
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
              {mutation.isPending ? <Trans>Sending…</Trans> : <Trans>Send invite</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
