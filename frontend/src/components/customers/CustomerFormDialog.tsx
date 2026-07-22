import { useMemo } from "react"
import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import type { CustomerResponse } from "@/lib/generated/types"
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
import { Textarea } from "@/components/ui/textarea"

type CustomerFormValues = {
  name: string
  company: string
  email: string
  phone: string
  address: string
  notes: string
}

function toFormValues(c?: CustomerResponse): CustomerFormValues {
  return {
    name: c?.name ?? "",
    company: c?.company ?? "",
    email: c?.email ?? "",
    phone: c?.phone ?? "",
    address: c?.address ?? "",
    notes: c?.notes ?? "",
  }
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // When provided, the dialog edits; otherwise it creates.
  customer?: CustomerResponse
}

/**
 * The create/edit form for customers, rendered in a dialog. It owns its
 * mutation so both the list and detail routes reuse it. Server-side 422s map
 * onto the fields via applyServerErrors; RHF handles the client-side required
 * rule. This is the house form pattern later entities copy.
 */
export function CustomerFormDialog({ open, onOpenChange, customer }: Props) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const isEdit = customer !== undefined

  const values = useMemo(() => toFormValues(customer), [customer])
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<CustomerFormValues>({ values })

  const mutation = useMutation({
    mutationFn: (v: CustomerFormValues) =>
      isEdit ? api.customers.update(customer.id, v) : api.customers.create(v),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.customers.all() })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {isEdit ? <Trans>Edit customer</Trans> : <Trans>New customer</Trans>}
          </DialogTitle>
          <DialogDescription>
            <Trans>Contact and notes for this customer.</Trans>
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={handleSubmit((v) => mutation.mutate(v))}
        >
          <div className="space-y-2">
            <Label htmlFor="name">
              <Trans>Name</Trans>
            </Label>
            <Input
              id="name"
              {...register("name", { required: t`This field is required.` })}
            />
            {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="company">
                <Trans>Company</Trans>
              </Label>
              <Input id="company" {...register("company")} />
              {errors.company && (
                <p className="text-sm text-destructive">{errors.company.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="phone">
                <Trans>Phone</Trans>
              </Label>
              <Input id="phone" {...register("phone")} />
              {errors.phone && <p className="text-sm text-destructive">{errors.phone.message}</p>}
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="email">
              <Trans>Email</Trans>
            </Label>
            <Input id="email" type="email" {...register("email")} />
            {errors.email && <p className="text-sm text-destructive">{errors.email.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="address">
              <Trans>Address</Trans>
            </Label>
            <Input id="address" {...register("address")} />
            {errors.address && <p className="text-sm text-destructive">{errors.address.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="notes">
              <Trans>Notes</Trans>
            </Label>
            <Textarea id="notes" {...register("notes")} />
            {errors.notes && <p className="text-sm text-destructive">{errors.notes.message}</p>}
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
