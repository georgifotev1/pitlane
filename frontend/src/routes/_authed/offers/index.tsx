import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { FileTextIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { formatMoney } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { PageHeader } from "@/components/layout/PageHeader"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { OfferStatusBadge, SendStatusBadge } from "@/components/offers/OfferStatusBadge"

const PAGE_SIZE = 20

// The board can be filtered to one lifecycle status or show all ("").
const STATUSES = ["", "draft", "sent", "accepted", "rejected", "expired"] as const
type StatusFilter = (typeof STATUSES)[number]

type OfferSearch = {
  page: number
  status: StatusFilter
}

export const Route = createFileRoute("/_authed/offers/")({
  // Typed, validated search params — the filter and page live in the URL so
  // they survive refresh and are shareable (ADR §26).
  validateSearch: (search: Record<string, unknown>): OfferSearch => {
    const page = Number(search.page)
    const status = STATUSES.includes(search.status as StatusFilter)
      ? (search.status as StatusFilter)
      : ""
    return { page: Number.isInteger(page) && page > 0 ? page : 1, status }
  },
  component: OffersBoard,
})

function OffersBoard() {
  const { t } = useLingui()
  const navigate = useNavigate({ from: Route.fullPath })
  const { page, status } = Route.useSearch()

  const query = useQuery({
    queryKey: queryKeys.offers.board({ page, pageSize: PAGE_SIZE, status }),
    queryFn: () => api.offers.listAll({ page, pageSize: PAGE_SIZE, status }),
    placeholderData: keepPreviousData,
  })

  const total = query.data?.metadata.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const offers = query.data?.offers ?? []
  // First run = success, nothing found, and no filter masking the list. This is
  // the moment to explain how an offer is born instead of showing an empty table.
  const isFirstRun = query.isSuccess && offers.length === 0 && !status

  function setStatus(next: StatusFilter) {
    navigate({ search: (prev) => ({ ...prev, status: next, page: 1 }) })
  }

  function goToPage(next: number) {
    navigate({ search: (prev) => ({ ...prev, page: next }) })
  }

  const filterLabel: Record<StatusFilter, React.ReactNode> = {
    "": <Trans>All</Trans>,
    draft: <Trans>Draft</Trans>,
    sent: <Trans>Sent</Trans>,
    accepted: <Trans>Accepted</Trans>,
    rejected: <Trans>Rejected</Trans>,
    expired: <Trans>Expired</Trans>,
  }

  return (
    <>
      <PageHeader
        title={<Trans>Offers</Trans>}
        description={<Trans>Repair quotes across the whole garage.</Trans>}
      />

      <div className="flex flex-wrap gap-1">
        {STATUSES.map((s) => (
          <Button
            key={s || "all"}
            size="sm"
            variant={status === s ? "default" : "outline"}
            onClick={() => setStatus(s)}
          >
            {filterLabel[s]}
          </Button>
        ))}
      </div>

      {isFirstRun && (
        <section className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-16 text-center">
          <FileTextIcon className="size-8 text-muted-foreground" />
          <div className="space-y-1">
            <p className="font-medium">
              <Trans>No offers yet</Trans>
            </p>
            <p className="max-w-sm text-sm text-muted-foreground">
              <Trans>
                Offers are written for a specific car. Open a customer, pick their car, and
                create your first offer there.
              </Trans>
            </p>
          </div>
          <Button
            render={
              <Link to="/customers" search={{ page: 1, search: "", archived: false }} />
            }
          >
            <Trans>Go to customers</Trans>
          </Button>
        </section>
      )}

      <section className={isFirstRun ? "hidden" : "rounded-lg border border-border"}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Status</Trans>
              </TableHead>
              <TableHead>
                <Trans>Car</Trans>
              </TableHead>
              <TableHead>
                <Trans>Customer</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Total</Trans>
              </TableHead>
              <TableHead>
                <Trans>Notes</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending &&
              Array.from({ length: 5 }, (_, i) => (
                <TableRow key={i} aria-hidden>
                  <TableCell>
                    <Skeleton className="h-5 w-20 rounded-4xl" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-32" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="ml-auto h-4 w-16" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-40" />
                  </TableCell>
                </TableRow>
              ))}
            {query.isError && (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-destructive">
                  <Trans>Could not load offers.</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isSuccess && offers.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                  <Trans>No offers with this status.</Trans>
                </TableCell>
              </TableRow>
            )}
            {offers.map((offer) => (
              <TableRow key={offer.id}>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-1.5">
                    <OfferStatusBadge status={offer.status} />
                    <SendStatusBadge status={offer.status} sendStatus={offer.sendStatus} />
                  </div>
                </TableCell>
                <TableCell className="font-medium">
                  {/* Offers are managed on the car's page, so the row links there. */}
                  <Link
                    to="/cars/$carId"
                    params={{ carId: offer.carId }}
                    className="underline-offset-4 hover:underline"
                    aria-label={t`Open car`}
                  >
                    {offer.carPlate}
                  </Link>
                </TableCell>
                <TableCell className="text-muted-foreground">{offer.customerName}</TableCell>
                <TableCell className="text-right tabular-nums font-medium">
                  {formatMoney(offer.totalCents)}
                </TableCell>
                <TableCell className="max-w-xs truncate text-muted-foreground">
                  {offer.notes || "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </section>

      <footer className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          <Trans>
            Page {page} of {totalPages} · {total} total
          </Trans>
        </span>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => goToPage(page - 1)}>
            <Trans>Previous</Trans>
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={page >= totalPages}
            onClick={() => goToPage(page + 1)}
          >
            <Trans>Next</Trans>
          </Button>
        </div>
      </footer>
    </>
  )
}
