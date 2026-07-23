import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import type { InvitationResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  invitation: InvitationResponse
}

/**
 * Confirms revoking a pending invitation. The invite link dies with the row and
 * the email becomes re-invitable.
 */
export function RevokeInviteDialog({ open, onOpenChange, invitation }: Props) {
  const qc = useQueryClient()

  const mutation = useMutation({
    mutationFn: () => api.users.invitations.revoke(invitation.id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.users.invitations() })
      onOpenChange(false)
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Revoke invitation</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>The link sent to {invitation.email} will stop working.</Trans>
          </DialogDescription>
        </DialogHeader>
        {mutation.isError && (
          <p className="text-sm text-destructive">
            <Trans>Could not revoke. Please try again.</Trans>
          </p>
        )}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={mutation.isPending}
          >
            <Trans>Cancel</Trans>
          </Button>
          <Button
            type="button"
            variant="destructive"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
          >
            {mutation.isPending ? <Trans>Revoking…</Trans> : <Trans>Revoke</Trans>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
