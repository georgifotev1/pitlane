import { useState } from "react"
import { createFileRoute, Link } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon, PencilIcon, PlayIcon, CheckCircleIcon, RotateCcwIcon } from "lucide-react"
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
import { RepairStatusBadge } from "@/components/repairs/RepairStatusBadge"
import { RepairFormDialog } from "@/components/repairs/RepairFormDialog"
import { CompleteRepairDialog } from "@/components/repairs/CompleteRepairDialog"
import { AttachmentsSection } from "@/components/attachments/AttachmentsSection"

export const Route = createFileRoute("/_authed/repairs/$repairId")({
  component: RepairDetail,
})

// KindLabel renders an item's kind through the catalog. A tiny component keeps
// the <Trans> macros static (extractable) without threading `t` around.
function KindLabel({ kind }: { kind: string }) {
  if (kind === "part") return <Trans>Part</Trans>
  if (kind === "labor") return <Trans>Labor</Trans>
  return <Trans>Other</Trans>
}

function RepairDetail() {
  const { t } = useLingui()
  const qc = useQueryClient()
  const { repairId } = Route.useParams()
  const [editing, setEditing] = useState(false)
  const [completing, setCompleting] = useState(false)
  const [actionError, setActionError] = useState("")

  const query = useQuery({
    queryKey: queryKeys.repairs.detail(repairId),
    queryFn: () => api.repairs.get(repairId),
    retry: false,
  })

  // The car supplies the plate for the header and the current odometer to
  // prefill the completion dialog. Dependent on the repair load.
  const carId = query.data?.carId
  const carQuery = useQuery({
    queryKey: queryKeys.cars.detail(carId ?? ""),
    queryFn: () => api.cars.get(carId!),
    enabled: !!carId,
  })

  const statusMutation = useMutation({
    mutationFn: (status: string) => api.repairs.setStatus(repairId, status),
    onMutate: () => setActionError(""),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.repairs.all() })
      await qc.invalidateQueries({ queryKey: queryKeys.repairs.detail(repairId) })
    },
    onError: (err) => {
      const code = err instanceof ProblemError ? err.code : undefined
      setActionError(errorCodeToMessage(code) || t`The action could not be completed.`)
    },
  })

  if (query.isPending) {
    return (
      <p className="p-6 text-sm text-muted-foreground">
        <Trans>Loading…</Trans>
      </p>
    )
  }

  if (query.isError || !query.data) {
    return (
      <main className="mx-auto max-w-3xl p-6">
        <p className="text-sm text-destructive">
          <Trans>Repair not found.</Trans>
        </p>
      </main>
    )
  }

  const repair = query.data
  const isOpen = repair.status === "open"
  const isInProgress = repair.status === "in_progress"
  const busy = statusMutation.isPending

  return (
    <main className="mx-auto flex min-h-svh max-w-3xl flex-col gap-6 p-6">
      {carId && (
        <Link
          to="/cars/$carId"
          params={{ carId }}
          className="inline-flex items-center gap-1 text-sm text-muted-foreground underline-offset-4 hover:underline"
        >
          <ArrowLeftIcon className="size-4" />
          <Trans>Back to car</Trans>
        </Link>
      )}

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
            <Trans>Repair</Trans>
            {carQuery.data && (
              <span className="text-muted-foreground">· {carQuery.data.plate}</span>
            )}
          </h1>
          <div className="mt-1 flex items-center gap-2">
            <RepairStatusBadge status={repair.status} />
            {repair.completedAt && (
              <span className="text-xs text-muted-foreground">
                <Trans>
                  Completed {new Date(repair.completedAt).toLocaleDateString("bg-BG")} at{" "}
                  {repair.mileage.toLocaleString("bg-BG")} km
                </Trans>
              </span>
            )}
          </div>
        </div>

        <div className="flex flex-wrap gap-1">
          {isOpen && (
            <>
              <Button size="sm" variant="ghost" onClick={() => setEditing(true)}>
                <PencilIcon />
                <Trans>Edit</Trans>
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => statusMutation.mutate("in_progress")}
              >
                <PlayIcon />
                <Trans>Start work</Trans>
              </Button>
            </>
          )}
          {isInProgress && (
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => statusMutation.mutate("open")}
            >
              <RotateCcwIcon />
              <Trans>Reopen</Trans>
            </Button>
          )}
          {(isOpen || isInProgress) && (
            <Button size="sm" onClick={() => setCompleting(true)}>
              <CheckCircleIcon />
              <Trans>Complete</Trans>
            </Button>
          )}
        </div>
      </header>

      {actionError && <p className="text-sm text-destructive">{actionError}</p>}

      <section className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Description</Trans>
              </TableHead>
              <TableHead>
                <Trans>Kind</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Qty</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Unit price</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Line total</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {repair.items.map((it) => (
              <TableRow key={it.id}>
                <TableCell className="font-medium">{it.description}</TableCell>
                <TableCell className="text-muted-foreground">
                  <KindLabel kind={it.kind} />
                </TableCell>
                <TableCell className="text-right tabular-nums">{it.quantity}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatMoney(it.unitPriceCents)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatMoney(it.lineTotalCents)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </section>

      <div className="flex flex-col gap-4 sm:flex-row sm:justify-between">
        {repair.notes ? (
          <p className="max-w-sm whitespace-pre-line text-sm text-muted-foreground">
            {repair.notes}
          </p>
        ) : (
          <span />
        )}
        <dl className="w-52 space-y-1 rounded-lg border border-border p-3 text-sm">
          <div className="flex justify-between">
            <dt className="text-muted-foreground">
              <Trans>Subtotal</Trans>
            </dt>
            <dd className="tabular-nums">{formatMoney(repair.subtotalCents)}</dd>
          </div>
          <div className="flex justify-between">
            <dt className="text-muted-foreground">
              <Trans>Tax</Trans> ({repair.taxRateBps / 100}%)
            </dt>
            <dd className="tabular-nums">{formatMoney(repair.taxCents)}</dd>
          </div>
          <div className="flex justify-between font-semibold">
            <dt>
              <Trans>Total</Trans>
            </dt>
            <dd className="tabular-nums">{formatMoney(repair.totalCents)}</dd>
          </div>
        </dl>
      </div>

      {isOpen && (
        <RepairFormDialog open={editing} onOpenChange={setEditing} repair={repair} />
      )}
      <CompleteRepairDialog
        open={completing}
        onOpenChange={setCompleting}
        repair={repair}
        currentMileage={carQuery.data?.mileage ?? 0}
      />

      <AttachmentsSection repairId={repair.id} />
    </main>
  )
}
