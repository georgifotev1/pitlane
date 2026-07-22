import { useMemo } from "react"
import { useFieldArray, useForm, useWatch } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, Trash2Icon } from "lucide-react"
import type { OfferResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { applyServerErrors } from "@/lib/formErrors"
import { queryKeys } from "@/lib/queryKeys"
import { formatMoney } from "@/lib/utils"
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
import { Textarea } from "@/components/ui/textarea"

// A line's unit price and quantity are held as strings in the form (so an empty
// field shows blank rather than "0") and parsed to cents / ints on submit and
// for the live totals. `unitPrice` is in MAJOR units (e.g. "45.00"); the server
// stores cents. Server validation keys use `unitPriceCents`, so onError remaps
// that suffix back onto this field.
type ItemFormValue = {
  kind: string
  description: string
  quantity: string
  unitPrice: string
}

type OfferFormValues = {
  notes: string
  taxRatePercent: string
  items: ItemFormValue[]
}

function centsFromMajor(major: string): number {
  const n = parseFloat(major)
  if (!Number.isFinite(n) || n < 0) return 0
  return Math.round(n * 100)
}

function parseQty(s: string): number {
  const n = parseInt(s, 10)
  return Number.isFinite(n) && n > 0 ? n : 0
}

// taxCents mirrors domain.TaxCents exactly (basis points, round half up) so the
// on-screen preview matches what the server will snapshot.
function taxCents(subtotalCents: number, bps: number): number {
  return Math.floor((subtotalCents * bps + 5000) / 10000)
}

function toFormValues(offer?: OfferResponse): OfferFormValues {
  if (!offer) {
    return {
      notes: "",
      taxRatePercent: "",
      items: [{ kind: "part", description: "", quantity: "1", unitPrice: "" }],
    }
  }
  return {
    notes: offer.notes,
    taxRatePercent: String(offer.taxRateBps / 100),
    items: offer.items.map((it) => ({
      kind: it.kind,
      description: it.description,
      quantity: String(it.quantity),
      unitPrice: (it.unitPriceCents / 100).toFixed(2),
    })),
  }
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The car this offer belongs to (needed to create).
  carId: string
  // When provided, the dialog edits a draft; otherwise it creates.
  offer?: OfferResponse
}

/**
 * The create/edit editor for draft offers. Line items are managed with
 * useFieldArray; useWatch drives a live totals preview that mirrors the server
 * math. On create the tax rate is snapshotted server-side from the tenant
 * default (so it is not shown); on edit it is an editable percentage. A sent
 * offer is immutable — this dialog is only opened for drafts.
 */
export function OfferFormDialog({ open, onOpenChange, carId, offer }: Props) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const isEdit = offer !== undefined

  const values = useMemo(() => toFormValues(offer), [offer])
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<OfferFormValues>({ values })

  const { fields, append, remove } = useFieldArray({ control, name: "items" })

  // Live totals preview. On create the tax rate is unknown (server snapshots
  // the tenant default), so only the subtotal is shown until first save.
  const watchedItems = useWatch({ control, name: "items" })
  const watchedPercent = useWatch({ control, name: "taxRatePercent" })
  const subtotal = (watchedItems ?? []).reduce(
    (sum, it) => sum + centsFromMajor(it.unitPrice) * parseQty(it.quantity),
    0,
  )
  const bps = isEdit ? Math.round((parseFloat(watchedPercent) || 0) * 100) : 0
  const tax = taxCents(subtotal, bps)
  const total = subtotal + tax

  const mutation = useMutation({
    mutationFn: (v: OfferFormValues) => {
      const items = v.items.map((it) => ({
        kind: it.kind,
        description: it.description,
        quantity: parseQty(it.quantity),
        unitPriceCents: centsFromMajor(it.unitPrice),
      }))
      if (isEdit) {
        return api.offers.update(offer.id, {
          notes: v.notes,
          taxRateBps: Math.round((parseFloat(v.taxRatePercent) || 0) * 100),
          items,
        })
      }
      return api.offers.create(carId, { notes: v.notes, items })
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.offers.all() })
      onOpenChange(false)
    },
    // The server validates `items.<i>.unitPriceCents` and `taxRateBps`; the
    // form fields are `unitPrice` and `taxRatePercent`. Remap so each error
    // lands on the right input.
    onError: (err) =>
      applyServerErrors(setError, err, (f) =>
        f === "taxRateBps" ? "taxRatePercent" : f.replace(/\.unitPriceCents$/, ".unitPrice"),
      ),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? <Trans>Edit offer</Trans> : <Trans>New offer</Trans>}
          </DialogTitle>
          <DialogDescription>
            <Trans>Line items and totals for this repair quote.</Trans>
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <div className="grid grid-cols-[1fr_5rem_7rem_2rem] items-center gap-2 text-xs font-medium text-muted-foreground">
              <span>
                <Trans>Description</Trans>
              </span>
              <span>
                <Trans>Qty</Trans>
              </span>
              <span>
                <Trans>Unit price</Trans>
              </span>
              <span className="sr-only">
                <Trans>Actions</Trans>
              </span>
            </div>

            {fields.map((field, i) => (
              <div key={field.id} className="space-y-1">
                <div className="grid grid-cols-[1fr_5rem_7rem_2rem] items-start gap-2">
                  <div className="space-y-1">
                    <Input
                      aria-label={t`Description`}
                      placeholder={t`e.g. Brake pads`}
                      {...register(`items.${i}.description`, {
                        required: t`This field is required.`,
                      })}
                    />
                    <select
                      aria-label={t`Kind`}
                      className="h-8 w-full rounded-lg border border-input bg-background px-2 text-xs"
                      {...register(`items.${i}.kind`)}
                    >
                      <option value="part">{t`Part`}</option>
                      <option value="labor">{t`Labor`}</option>
                      <option value="other">{t`Other`}</option>
                    </select>
                  </div>
                  <Input
                    aria-label={t`Quantity`}
                    type="number"
                    inputMode="numeric"
                    min={1}
                    {...register(`items.${i}.quantity`)}
                  />
                  <Input
                    aria-label={t`Unit price`}
                    type="number"
                    inputMode="decimal"
                    step="0.01"
                    min={0}
                    {...register(`items.${i}.unitPrice`)}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t`Remove line`}
                    disabled={fields.length === 1}
                    onClick={() => remove(i)}
                  >
                    <Trash2Icon />
                  </Button>
                </div>
                {errors.items?.[i]?.description && (
                  <p className="text-sm text-destructive">
                    {errors.items[i]?.description?.message}
                  </p>
                )}
                {errors.items?.[i]?.quantity && (
                  <p className="text-sm text-destructive">{errors.items[i]?.quantity?.message}</p>
                )}
                {errors.items?.[i]?.unitPrice && (
                  <p className="text-sm text-destructive">{errors.items[i]?.unitPrice?.message}</p>
                )}
              </div>
            ))}

            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => append({ kind: "part", description: "", quantity: "1", unitPrice: "" })}
            >
              <PlusIcon />
              <Trans>Add line</Trans>
            </Button>
          </div>

          {isEdit && (
            <div className="w-40 space-y-2">
              <Label htmlFor="taxRatePercent">
                <Trans>Tax rate (%)</Trans>
              </Label>
              <Input
                id="taxRatePercent"
                type="number"
                inputMode="decimal"
                step="0.01"
                min={0}
                {...register("taxRatePercent")}
              />
              {errors.taxRatePercent && (
                <p className="text-sm text-destructive">{errors.taxRatePercent.message}</p>
              )}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="notes">
              <Trans>Notes</Trans>
            </Label>
            <Textarea id="notes" rows={2} {...register("notes")} />
            {errors.notes && <p className="text-sm text-destructive">{errors.notes.message}</p>}
          </div>

          {/* Live totals preview — mirrors the server math. */}
          <dl className="ml-auto w-52 space-y-1 rounded-lg border border-border p-3 text-sm">
            <div className="flex justify-between">
              <dt className="text-muted-foreground">
                <Trans>Subtotal</Trans>
              </dt>
              <dd className="tabular-nums">{formatMoney(subtotal)}</dd>
            </div>
            {isEdit ? (
              <>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">
                    <Trans>Tax</Trans>
                  </dt>
                  <dd className="tabular-nums">{formatMoney(tax)}</dd>
                </div>
                <div className="flex justify-between font-semibold">
                  <dt>
                    <Trans>Total</Trans>
                  </dt>
                  <dd className="tabular-nums">{formatMoney(total)}</dd>
                </div>
              </>
            ) : (
              <p className="text-xs text-muted-foreground">
                <Trans>Tax and total are calculated on save.</Trans>
              </p>
            )}
          </dl>

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
