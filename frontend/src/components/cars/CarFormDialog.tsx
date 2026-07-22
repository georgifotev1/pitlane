import { useMemo } from "react"
import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import type { CarResponse } from "@/lib/generated/types"
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

// year/mileage are held as strings in the form (so an empty field shows blank
// rather than "0") and converted to numbers on submit; 0 means "unknown".
type CarFormValues = {
  plate: string
  vin: string
  make: string
  model: string
  year: string
  mileage: string
}

function toFormValues(c?: CarResponse): CarFormValues {
  return {
    plate: c?.plate ?? "",
    vin: c?.vin ?? "",
    make: c?.make ?? "",
    model: c?.model ?? "",
    year: c?.year ? String(c.year) : "",
    mileage: c?.mileage ? String(c.mileage) : "",
  }
}

function toInt(s: string): number {
  const n = parseInt(s, 10)
  return Number.isFinite(n) ? n : 0
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The customer this car belongs to (needed to create).
  customerId: string
  // When provided, the dialog edits; otherwise it creates.
  car?: CarResponse
}

/**
 * The create/edit form for cars, mirroring CustomerFormDialog: it owns its
 * mutation, maps server 422s onto fields via applyServerErrors (a duplicate
 * plate lands on the `plate` field), and RHF handles the client-side required
 * rule for the plate.
 */
export function CarFormDialog({ open, onOpenChange, customerId, car }: Props) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const isEdit = car !== undefined

  const values = useMemo(() => toFormValues(car), [car])
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<CarFormValues>({ values })

  const mutation = useMutation({
    mutationFn: (v: CarFormValues) => {
      const payload = {
        plate: v.plate,
        vin: v.vin,
        make: v.make,
        model: v.model,
        year: toInt(v.year),
        mileage: toInt(v.mileage),
      }
      return isEdit ? api.cars.update(car.id, payload) : api.cars.create(customerId, payload)
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.cars.all() })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isEdit ? <Trans>Edit car</Trans> : <Trans>New car</Trans>}
          </DialogTitle>
          <DialogDescription>
            <Trans>Vehicle details and mileage.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <Label htmlFor="plate">
              <Trans>Plate</Trans>
            </Label>
            <Input id="plate" {...register("plate", { required: t`This field is required.` })} />
            {errors.plate && <p className="text-sm text-destructive">{errors.plate.message}</p>}
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="make">
                <Trans>Make</Trans>
              </Label>
              <Input id="make" {...register("make")} />
              {errors.make && <p className="text-sm text-destructive">{errors.make.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="model">
                <Trans>Model</Trans>
              </Label>
              <Input id="model" {...register("model")} />
              {errors.model && <p className="text-sm text-destructive">{errors.model.message}</p>}
            </div>
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="year">
                <Trans>Year</Trans>
              </Label>
              <Input id="year" type="number" inputMode="numeric" {...register("year")} />
              {errors.year && <p className="text-sm text-destructive">{errors.year.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="mileage">
                <Trans>Mileage</Trans>
              </Label>
              <Input id="mileage" type="number" inputMode="numeric" {...register("mileage")} />
              {errors.mileage && (
                <p className="text-sm text-destructive">{errors.mileage.message}</p>
              )}
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="vin">
              <Trans>VIN</Trans>
            </Label>
            <Input id="vin" {...register("vin")} />
            {errors.vin && <p className="text-sm text-destructive">{errors.vin.message}</p>}
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
