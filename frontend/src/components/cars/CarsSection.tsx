import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, PencilIcon, ArchiveIcon } from "lucide-react"
import type { CarResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { CarFormDialog } from "@/components/cars/CarFormDialog"
import { ArchiveCarDialog } from "@/components/cars/ArchiveCarDialog"

// A customer has few cars, so we fetch a generous single page and render them
// all — no pagination/search UI. The API still supports both (pattern parity);
// this component just doesn't need them. State is local (not URL search params)
// because cars are nested inside the customer detail route, not a route of
// their own.
const PAGE_SIZE = 100

function formatNumber(n: number): string {
  return n.toLocaleString("bg-BG")
}

export function CarsSection({ customerId }: { customerId: string }) {
  const { t } = useLingui()
  const [showArchived, setShowArchived] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<CarResponse | undefined>()
  const [archiving, setArchiving] = useState<CarResponse | undefined>()

  const listQuery = { page: 1, pageSize: PAGE_SIZE, search: "", archived: showArchived }
  const query = useQuery({
    queryKey: queryKeys.cars.listForCustomer(customerId, listQuery),
    queryFn: () => api.cars.list(customerId, listQuery),
  })

  const cars = query.data?.cars ?? []

  return (
    <section className="flex flex-col gap-4">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">
            <Trans>Cars</Trans>
          </h2>
          <p className="text-sm text-muted-foreground">
            <Trans>Vehicles for this customer.</Trans>
          </p>
        </div>
        <div className="flex items-center gap-4">
          <Label className="gap-2">
            <input
              type="checkbox"
              checked={showArchived}
              onChange={(e) => setShowArchived(e.target.checked)}
              className="size-4 rounded border-input"
            />
            <Trans>Show archived</Trans>
          </Label>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <PlusIcon />
            <Trans>New car</Trans>
          </Button>
        </div>
      </header>

      <div className="rounded-lg border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Plate</Trans>
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
              <TableHead>
                <Trans>Mileage</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Actions</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending && (
              <TableRow>
                <TableCell colSpan={6} className="py-8 text-center text-muted-foreground">
                  <Trans>Loading…</Trans>
                </TableCell>
              </TableRow>
            )}
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
                  <Trans>No cars yet.</Trans>
                </TableCell>
              </TableRow>
            )}
            {cars.map((car) => (
              <TableRow key={car.id}>
                <TableCell className="font-medium">
                  {car.plate}
                  {car.archivedAt && (
                    <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                      <Trans>Archived</Trans>
                    </span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">{car.make || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{car.model || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{car.year || "—"}</TableCell>
                <TableCell className="text-muted-foreground">
                  {car.mileage ? formatNumber(car.mileage) : "—"}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => setEditing(car)}
                      aria-label={t`Edit`}
                    >
                      <PencilIcon />
                    </Button>
                    {!car.archivedAt && (
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        onClick={() => setArchiving(car)}
                        aria-label={t`Archive`}
                      >
                        <ArchiveIcon />
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <CarFormDialog open={createOpen} onOpenChange={setCreateOpen} customerId={customerId} />
      <CarFormDialog
        open={editing !== undefined}
        onOpenChange={(open) => !open && setEditing(undefined)}
        customerId={customerId}
        car={editing}
      />
      {archiving && (
        <ArchiveCarDialog
          open={archiving !== undefined}
          onOpenChange={(open) => !open && setArchiving(undefined)}
          car={archiving}
        />
      )}
    </section>
  )
}
