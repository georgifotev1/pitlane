import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { WrenchIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { formatMoney, isInteractiveTarget } from "@/lib/utils"
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
import { RepairStatusBadge } from "@/components/repairs/RepairStatusBadge"

const PAGE_SIZE = 20

// The board can be filtered to one status or show all ("").
const STATUSES = ["", "open", "in_progress", "completed"] as const
type StatusFilter = (typeof STATUSES)[number]

type RepairSearch = {
  page: number
  status: StatusFilter
}

export const Route = createFileRoute("/_authed/repairs/")({
  // Typed, validated search params — the filter and page live in the URL so
  // they survive refresh and are shareable (ADR §26).
  validateSearch: (search: Record<string, unknown>): RepairSearch => {
    const page = Number(search.page)
    const status = STATUSES.includes(search.status as StatusFilter)
      ? (search.status as StatusFilter)
      : ""
    return { page: Number.isInteger(page) && page > 0 ? page : 1, status }
  },
  component: RepairsBoard,
})

function RepairsBoard() {
  const { t } = useLingui()
  const navigate = useNavigate({ from: Route.fullPath })
  const { page, status } = Route.useSearch()

  const query = useQuery({
    queryKey: queryKeys.repairs.list({ page, pageSize: PAGE_SIZE, status }),
    queryFn: () => api.repairs.list({ page, pageSize: PAGE_SIZE, status }),
    placeholderData: keepPreviousData,
  })

  const total = query.data?.metadata.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const repairs = query.data?.repairs ?? []
  // First run = success, nothing found, and no filter masking the list. This is
  // the moment to explain where repairs come from instead of an empty table.
  const isFirstRun = query.isSuccess && repairs.length === 0 && !status

  function setStatus(next: StatusFilter) {
    navigate({ search: (prev) => ({ ...prev, status: next, page: 1 }) })
  }

  function goToPage(next: number) {
    navigate({ search: (prev) => ({ ...prev, page: next }) })
  }

  const filterLabel: Record<StatusFilter, React.ReactNode> = {
    "": <Trans>All</Trans>,
    open: <Trans>Open</Trans>,
    in_progress: <Trans>In progress</Trans>,
    completed: <Trans>Completed</Trans>,
  }

  return (
    <>
      <PageHeader
        title={<Trans>Repairs</Trans>}
        description={<Trans>Jobs across the whole garage. Convert a sent offer to start one.</Trans>}
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
          <WrenchIcon className="size-8 text-muted-foreground" />
          <div className="space-y-1">
            <p className="font-medium">
              <Trans>No repairs yet</Trans>
            </p>
            <p className="max-w-sm text-sm text-muted-foreground">
              <Trans>
                A repair starts when you accept a sent offer. Open the offers board to send
                or accept one.
              </Trans>
            </p>
          </div>
          <Button nativeButton={false} render={<Link to="/offers" search={{ page: 1, status: "" }} />}>
            <Trans>Go to offers</Trans>
          </Button>
        </section>
      )}

      <section className={isFirstRun ? "hidden" : "rounded-lg border border-border"}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Car</Trans>
              </TableHead>
              <TableHead>
                <Trans>Customer</Trans>
              </TableHead>
              <TableHead>
                <Trans>Status</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Total</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending &&
              Array.from({ length: 5 }, (_, i) => (
                <TableRow key={i} aria-hidden>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-32" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-20 rounded-4xl" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="ml-auto h-4 w-16" />
                  </TableCell>
                </TableRow>
              ))}
            {query.isError && (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-destructive">
                  <Trans>Could not load repairs.</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isSuccess && repairs.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                  <Trans>No repairs with this status.</Trans>
                </TableCell>
              </TableRow>
            )}
            {repairs.map((r) => (
              <TableRow
                key={r.id}
                className="cursor-pointer"
                role="link"
                tabIndex={0}
                aria-label={t`Open repair`}
                onClick={(e) => {
                  if (isInteractiveTarget(e.target)) return
                  navigate({ to: "/repairs/$repairId", params: { repairId: r.id } })
                }}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return
                  e.preventDefault()
                  navigate({ to: "/repairs/$repairId", params: { repairId: r.id } })
                }}
              >
                <TableCell className="font-medium">{r.carPlate}</TableCell>
                <TableCell className="text-muted-foreground">{r.customerName}</TableCell>
                <TableCell>
                  <RepairStatusBadge status={r.status} />
                </TableCell>
                <TableCell className="text-right tabular-nums font-medium">
                  {formatMoney(r.totalCents)}
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
