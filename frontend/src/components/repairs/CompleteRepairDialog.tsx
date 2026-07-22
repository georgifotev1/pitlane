import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { CheckCircleIcon } from "lucide-react"
import type { RepairResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { applyServerErrors } from "@/lib/formErrors"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type CompleteFormValues = { mileage: string }

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  repair: RepairResponse
  // The car's current odometer, prefilled as the reading to confirm/adjust.
  currentMileage: number
}

/**
 * Completes a repair, capturing the odometer reading at hand-back. The reading
 * is written onto the car (advancing its mileage) and freezes the repair. It is
 * the sole path to the `completed` state.
 */
export function CompleteRepairDialog({ open, onOpenChange, repair, currentMileage }: Props) {
  const qc = useQueryClient()
  const { t } = useLingui()

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<CompleteFormValues>({
    values: { mileage: currentMileage ? String(currentMileage) : "" },
  })

  const mutation = useMutation({
    mutationFn: (v: CompleteFormValues) => {
      const mileage = parseInt(v.mileage, 10)
      return api.repairs.complete(repair.id, Number.isFinite(mileage) && mileage > 0 ? mileage : 0)
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.repairs.all() })
      await qc.invalidateQueries({ queryKey: queryKeys.repairs.detail(repair.id) })
      // The car's mileage advanced too — drop its cached reads.
      await qc.invalidateQueries({ queryKey: queryKeys.cars.all() })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Complete repair</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              Record the car's odometer reading. This finishes the repair and updates the car's
              mileage.
            </Trans>
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <Label htmlFor="mileage">
              <Trans>Mileage (km)</Trans>
            </Label>
            <Input
              id="mileage"
              type="number"
              inputMode="numeric"
              min={0}
              placeholder={t`e.g. 145000`}
              {...register("mileage")}
            />
            {errors.mileage && <p className="text-sm text-destructive">{errors.mileage.message}</p>}
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
              <CheckCircleIcon />
              {mutation.isPending ? <Trans>Completing…</Trans> : <Trans>Complete</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
