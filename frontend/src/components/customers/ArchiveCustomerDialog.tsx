import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import type { CustomerResponse } from "@/lib/generated/types"
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
  customer: CustomerResponse
  // Optional hook after a successful archive (e.g. navigate away from detail).
  onArchived?: () => void
}

/**
 * Confirmation dialog for the soft-delete (archive) action. Archiving is
 * reversible-by-design (archived_at), but destructive to the default view, so
 * it always confirms.
 */
export function ArchiveCustomerDialog({ open, onOpenChange, customer, onArchived }: Props) {
  const qc = useQueryClient()

  const mutation = useMutation({
    mutationFn: () => api.customers.archive(customer.id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.customers.all() })
      onOpenChange(false)
      onArchived?.()
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Archive customer</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              This hides {customer.name} from the active list. You can still view archived customers
              with the filter.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        {mutation.isError && (
          <p className="text-sm text-destructive">
            <Trans>Could not archive. Please try again.</Trans>
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
            {mutation.isPending ? <Trans>Archiving…</Trans> : <Trans>Archive</Trans>}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
