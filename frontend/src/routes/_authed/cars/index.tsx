import { useState } from "react"
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { CarIcon } from "lucide-react"
import { api } from "@/lib/api"
import { isInteractiveTarget } from "@/lib/utils"
import { queryKeys } from "@/lib/queryKeys"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
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

const PAGE_SIZE = 20

type CarSearch = {
  page: number
  search: string
  archived: boolean
}

export const Route = createFileRoute("/_authed/cars/")({
  // Typed, validated search params — pagination and filters live in the URL so
  // they survive refresh and are shareable (ADR §26).
  validateSearch: (search: Record<string, unknown>): CarSearch => {
    const page = Number(search.page)
    return {
      page: Number.isInteger(page) && page > 0 ? page : 1,
      search: typeof search.search === "string" ? search.search : "",
      archived: search.archived === true || search.archived === "true",
    }
  },
  component: CarsBoard,
})

function CarsBoard() {
  const { t } = useLingui()
  const navigate = useNavigate({ from: Route.fullPath })
  const { page, search, archived } = Route.useSearch()

  const [searchInput, setSearchInput] = useState(search)

  const query = useQuery({
    queryKey: queryKeys.cars.board({ page, pageSize: PAGE_SIZE, search, archived }),
    queryFn: () => api.cars.listAll({ page, pageSize: PAGE_SIZE, search, archived }),
    placeholderData: keepPreviousData,
  })

  const total = query.data?.metadata.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const cars = query.data?.cars ?? []
  // First run = success, nothing found, and no filters masking the list. This
  // is the moment to explain where cars come from instead of an empty table.
  const isFirstRun = query.isSuccess && cars.length === 0 && !search && !archived

  function submitSearch(e: React.FormEvent) {
    e.preventDefault()
    navigate({ search: (prev) => ({ ...prev, search: searchInput.trim(), page: 1 }) })
  }

  function toggleArchived(next: boolean) {
    navigate({ search: (prev) => ({ ...prev, archived: next, page: 1 }) })
  }

  function goToPage(next: number) {
    navigate({ search: (prev) => ({ ...prev, page: next }) })
  }

  return (
    <>
      <PageHeader
        title={<Trans>Cars</Trans>}
        description={
          <Trans>Every vehicle in the garage. Open one for its offers and service history.</Trans>
        }
      />

      <div className="flex flex-wrap items-end justify-between gap-4">
        <form onSubmit={submitSearch} className="flex items-end gap-2">
          <div className="space-y-2">
            <Label htmlFor="search">
              <Trans>Search</Trans>
            </Label>
            <Input
              id="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder={t`Plate, VIN, make, model`}
              className="w-64"
            />
          </div>
          <Button type="submit" variant="outline">
            <Trans>Search</Trans>
          </Button>
        </form>
        <Label className="gap-2">
          <input
            type="checkbox"
            checked={archived}
            onChange={(e) => toggleArchived(e.target.checked)}
            className="size-4 rounded border-input"
          />
          <Trans>Show archived</Trans>
        </Label>
      </div>

      {isFirstRun && (
        <section className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-16 text-center">
          <CarIcon className="size-8 text-muted-foreground" />
          <div className="space-y-1">
            <p className="font-medium">
              <Trans>No cars yet</Trans>
            </p>
            <p className="max-w-sm text-sm text-muted-foreground">
              <Trans>
                Cars belong to a customer. Open a customer and add their first car there.
              </Trans>
            </p>
          </div>
          <Button
            nativeButton={false}
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
                <Trans>Plate</Trans>
              </TableHead>
              <TableHead>
                <Trans>Customer</Trans>
              </TableHead>
              <TableHead>
                <Trans>Make</Trans>
              </TableHead>
              <TableHead>
                <Trans>Model</Trans>
              </TableHead>
              <TableHead>
                <Trans>Year</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Mileage</Trans>
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
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-20" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-12" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="ml-auto h-4 w-16" />
                  </TableCell>
                </TableRow>
              ))}
            {query.isError && (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-destructive">
                  <Trans>Could not load cars.</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isSuccess && cars.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  <Trans>No cars found.</Trans>
                </TableCell>
              </TableRow>
            )}
            {cars.map((car) => (
              <TableRow
                key={car.id}
                className="cursor-pointer"
                role="link"
                tabIndex={0}
                aria-label={t`Open car`}
                onClick={(e) => {
                  if (isInteractiveTarget(e.target, e.currentTarget)) return
                  navigate({ to: "/cars/$carId", params: { carId: car.id } })
                }}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return
                  e.preventDefault()
                  navigate({ to: "/cars/$carId", params: { carId: car.id } })
                }}
              >
                <TableCell className="font-medium">
                  {car.plate}
                  {car.archivedAt && (
                    <Badge variant="secondary" className="ml-2">
                      <Trans>Archived</Trans>
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">{car.customerName}</TableCell>
                <TableCell className="text-muted-foreground">{car.make || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{car.model || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{car.year || "—"}</TableCell>
                <TableCell className="text-right tabular-nums text-muted-foreground">
                  {car.mileage ? car.mileage.toLocaleString("bg-BG") : "—"}
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
