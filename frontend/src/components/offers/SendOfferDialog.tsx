import { useForm } from "react-hook-form"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { SendIcon } from "lucide-react"
import type { OfferResponse } from "@/lib/generated/types"
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

type SendFormValues = { recipient: string }

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The offer to send. Undefined while closed, so the form only initializes
  // when an offer is selected.
  offer?: OfferResponse
  // Prefill for the initial send — the customer's email. On a retry the offer's
  // own sentTo (the address used last time) takes precedence.
  defaultRecipient: string
}

/**
 * Emails an offer's PDF to a customer. Sending freezes the offer (draft → sent)
 * and enqueues the delivery job server-side; this dialog is opened both for the
 * initial send of a draft and to retry a failed delivery. The recipient is
 * prefilled but editable, and validated server-side (422 → field error).
 */
export function SendOfferDialog({ open, onOpenChange, offer, defaultRecipient }: Props) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const isRetry = offer?.sendStatus === "failed"

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SendFormValues>({
    values: { recipient: offer?.sentTo || defaultRecipient },
  })

  const mutation = useMutation({
    mutationFn: (v: SendFormValues) => {
      if (!offer) throw new Error("no offer")
      return api.offers.send(offer.id, v.recipient.trim())
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.offers.all() })
      onOpenChange(false)
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isRetry ? <Trans>Retry sending</Trans> : <Trans>Send offer</Trans>}
          </DialogTitle>
          <DialogDescription>
            <Trans>
              The offer PDF will be emailed to the customer. Once sent, the offer
              can no longer be edited.
            </Trans>
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={handleSubmit((v) => mutation.mutate(v))}>
          <div className="space-y-2">
            <Label htmlFor="recipient">
              <Trans>Recipient email</Trans>
            </Label>
            <Input
              id="recipient"
              type="email"
              inputMode="email"
              autoComplete="email"
              placeholder={t`customer@example.com`}
              {...register("recipient")}
            />
            {errors.recipient && (
              <p className="text-sm text-destructive">{errors.recipient.message}</p>
            )}
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
              <SendIcon />
              {mutation.isPending ? <Trans>Sending…</Trans> : <Trans>Send</Trans>}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
