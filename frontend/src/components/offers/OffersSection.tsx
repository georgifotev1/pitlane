import { useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, PencilIcon, SendIcon, FileTextIcon, WrenchIcon } from "lucide-react"
import type { OfferResponse } from "@/lib/generated/types"
import { api, ProblemError } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { errorCodeToMessage } from "@/lib/errorCodes"
import { formatMoney } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { OfferFormDialog } from "@/components/offers/OfferFormDialog"
import { OfferPdfDialog } from "@/components/offers/OfferPdfDialog"
import { SendOfferDialog } from "@/components/offers/SendOfferDialog"

// A car has few offers, so we fetch a generous single page and render them all
// — no pagination UI (the API still supports it, matching CarsSection).
const PAGE_SIZE = 100

function StatusBadge({ status }: { status: string }) {
  // Amber-ish for draft, neutral for terminal states; kept simple with the
  // existing muted palette so no new tokens are needed.
  const tone =
    status === "accepted"
      ? "bg-primary/10 text-primary"
      : status === "rejected" || status === "expired"
        ? "bg-destructive/10 text-destructive"
        : "bg-muted text-muted-foreground"
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${tone}`}>
      {status === "draft" && <Trans>Draft</Trans>}
      {status === "sent" && <Trans>Sent</Trans>}
      {status === "accepted" && <Trans>Accepted</Trans>}
      {status === "rejected" && <Trans>Rejected</Trans>}
      {status === "expired" && <Trans>Expired</Trans>}
    </span>
  )
}

// SendStatusBadge surfaces the email-delivery lifecycle (send_status) for offers
// that have been sent. It is meaningless on a draft, so nothing renders there.
function SendStatusBadge({ status, sendStatus }: { status: string; sendStatus: string }) {
  if (status === "draft") return null
  const tone =
    sendStatus === "sent"
      ? "bg-primary/10 text-primary"
      : sendStatus === "failed"
        ? "bg-destructive/10 text-destructive"
        : "bg-muted text-muted-foreground"
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${tone}`}>
      {sendStatus === "pending" && <Trans>Sending…</Trans>}
      {sendStatus === "sent" && <Trans>Emailed</Trans>}
      {sendStatus === "failed" && <Trans>Send failed</Trans>}
    </span>
  )
}

export function OffersSection({ carId, defaultRecipient }: { carId: string; defaultRecipient: string }) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<OfferResponse | undefined>()
  const [previewing, setPreviewing] = useState<OfferResponse | undefined>()
  const [sending, setSending] = useState<OfferResponse | undefined>()
  const [statusError, setStatusError] = useState<string>("")

  const listQuery = { page: 1, pageSize: PAGE_SIZE }
  const query = useQuery({
    queryKey: queryKeys.offers.listForCar(carId, listQuery),
    queryFn: () => api.offers.list(carId, listQuery),
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.offers.setStatus(id, status),
    onMutate: () => setStatusError(""),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.offers.all() })
    },
    onError: (err) => {
      const code = err instanceof ProblemError ? err.code : undefined
      setStatusError(errorCodeToMessage(code) || t`The action could not be completed.`)
    },
  })

  // Accepting a sent offer converts it into a repair (the sole accept path).
  // On success we jump straight to the new repair so the mechanic can start work.
  const convertMutation = useMutation({
    mutationFn: (id: string) => api.offers.accept(id),
    onMutate: () => setStatusError(""),
    onSuccess: async (repair) => {
      await qc.invalidateQueries({ queryKey: queryKeys.offers.all() })
      await qc.invalidateQueries({ queryKey: queryKeys.repairs.all() })
      await navigate({ to: "/repairs/$repairId", params: { repairId: repair.id } })
    },
    onError: (err) => {
      const code = err instanceof ProblemError ? err.code : undefined
      setStatusError(errorCodeToMessage(code) || t`The action could not be completed.`)
    },
  })

  const offers = query.data?.offers ?? []

  return (
    <section className="flex flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">
            <Trans>Offers</Trans>
          </h2>
          <p className="text-sm text-muted-foreground">
            <Trans>Repair quotes for this car.</Trans>
          </p>
        </div>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <PlusIcon />
          <Trans>New offer</Trans>
        </Button>
      </header>

      {statusError && <p className="text-sm text-destructive">{statusError}</p>}

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Status</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Total</Trans>
              </TableHead>
              <TableHead>
                <Trans>Notes</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Actions</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending && (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                  <Trans>Loading…</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isError && (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-destructive">
                  <Trans>Could not load offers.</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isSuccess && offers.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                  <Trans>No offers yet.</Trans>
                </TableCell>
              </TableRow>
            )}
            {offers.map((offer) => {
              const busy = statusMutation.isPending || convertMutation.isPending
              return (
                <TableRow key={offer.id}>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <StatusBadge status={offer.status} />
                      <SendStatusBadge status={offer.status} sendStatus={offer.sendStatus} />
                    </div>
                  </TableCell>
                  <TableCell className="text-right tabular-nums font-medium">
                    {formatMoney(offer.totalCents)}
                  </TableCell>
                  <TableCell className="max-w-xs truncate text-muted-foreground">
                    {offer.notes || "—"}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        aria-label={t`Preview PDF`}
                        onClick={() => setPreviewing(offer)}
                      >
                        <FileTextIcon />
                      </Button>
                      {offer.status === "draft" && (
                        <>
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            aria-label={t`Edit`}
                            onClick={() => setEditing(offer)}
                          >
                            <PencilIcon />
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => setSending(offer)}
                          >
                            <SendIcon />
                            <Trans>Send</Trans>
                          </Button>
                        </>
                      )}
                      {offer.status === "sent" && (
                        <>
                          {offer.sendStatus === "failed" && (
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setSending(offer)}
                            >
                              <SendIcon />
                              <Trans>Retry</Trans>
                            </Button>
                          )}
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={busy}
                            onClick={() => convertMutation.mutate(offer.id)}
                          >
                            <WrenchIcon />
                            <Trans>Convert to repair</Trans>
                          </Button>
                          <Button
                            size="sm"
                            variant="destructive"
                            disabled={busy}
                            onClick={() =>
                              statusMutation.mutate({ id: offer.id, status: "rejected" })
                            }
                          >
                            <Trans>Reject</Trans>
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={busy}
                            onClick={() =>
                              statusMutation.mutate({ id: offer.id, status: "expired" })
                            }
                          >
                            <Trans>Expire</Trans>
                          </Button>
                        </>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>

      <OfferFormDialog open={createOpen} onOpenChange={setCreateOpen} carId={carId} />
      <OfferFormDialog
        open={editing !== undefined}
        onOpenChange={(open) => !open && setEditing(undefined)}
        carId={carId}
        offer={editing}
      />
      <OfferPdfDialog
        open={previewing !== undefined}
        onOpenChange={(open) => !open && setPreviewing(undefined)}
        offerId={previewing?.id}
      />
      <SendOfferDialog
        open={sending !== undefined}
        onOpenChange={(open) => !open && setSending(undefined)}
        offer={sending}
        defaultRecipient={defaultRecipient}
      />
    </section>
  )
}
