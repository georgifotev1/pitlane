import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import type { CarResponse } from "@/lib/generated/types"
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
  car: CarResponse
}

/**
 * Confirmation dialog for archiving (soft-deleting) a car. Mirrors
 * ArchiveCustomerDialog: reversible-by-design, but destructive to the default
 * view, so it always confirms. Archiving frees the plate for re-registration.
 */
export function ArchiveCarDialog({ open, onOpenChange, car }: Props) {
  const qc = useQueryClient()

  const mutation = useMutation({
    mutationFn: () => api.cars.archive(car.id),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.cars.all() })
      onOpenChange(false)
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Archive car</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              This hides {car.plate} from the active list and frees its plate. You can still show
              archived cars with the toggle.
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
