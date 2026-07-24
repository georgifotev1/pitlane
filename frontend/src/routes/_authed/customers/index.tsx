import { useState } from "react"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, PencilIcon, ArchiveIcon, UsersIcon } from "lucide-react"
import type { CustomerResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { isInteractiveTarget } from "@/lib/utils"
import { queryKeys } from "@/lib/queryKeys"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { CustomerFormDialog } from "@/components/customers/CustomerFormDialog"
import { ArchiveCustomerDialog } from "@/components/customers/ArchiveCustomerDialog"
import { PageHeader } from "@/components/layout/PageHeader"

const PAGE_SIZE = 20

type CustomerSearch = {
  page: number
  search: string
  archived: boolean
}

export const Route = createFileRoute("/_authed/customers/")({
  // Typed, validated search params — pagination and filters live in the URL so
  // they survive refresh and are shareable (ADR §26).
  validateSearch: (search: Record<string, unknown>): CustomerSearch => {
    const page = Number(search.page)
    return {
      page: Number.isInteger(page) && page > 0 ? page : 1,
      search: typeof search.search === "string" ? search.search : "",
      archived: search.archived === true || search.archived === "true",
    }
  },
  component: CustomersList,
})

function CustomersList() {
  const { t } = useLingui()
  const navigate = useNavigate({ from: Route.fullPath })
  const { page, search, archived } = Route.useSearch()

  const [searchInput, setSearchInput] = useState(search)
  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<CustomerResponse | undefined>()
  const [archiving, setArchiving] = useState<CustomerResponse | undefined>()

  const query = useQuery({
    queryKey: queryKeys.customers.list({ page, pageSize: PAGE_SIZE, search, archived }),
    queryFn: () => api.customers.list({ page, pageSize: PAGE_SIZE, search, archived }),
    placeholderData: keepPreviousData,
  })

  const total = query.data?.metadata.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const customers = query.data?.customers ?? []
  // First run = success, nothing found, and no filters masking the list. This
  // is the moment for a welcome panel instead of an empty table.
  const isFirstRun = query.isSuccess && customers.length === 0 && !search && !archived

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
        title={<Trans>Customers</Trans>}
        description={<Trans>People and companies your garage works with.</Trans>}
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon />
            <Trans>New customer</Trans>
          </Button>
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
              placeholder={t`Name, company, email, phone`}
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
          <UsersIcon className="size-8 text-muted-foreground" />
          <div className="space-y-1">
            <p className="font-medium">
              <Trans>No customers yet</Trans>
            </p>
            <p className="text-sm text-muted-foreground">
              <Trans>Add your first customer to get the garage rolling.</Trans>
            </p>
          </div>
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon />
            <Trans>New customer</Trans>
          </Button>
        </section>
      )}
      <section className={isFirstRun ? "hidden" : "rounded-lg border border-border"}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                <Trans>Name</Trans>
              </TableHead>
              <TableHead>
                <Trans>Company</Trans>
              </TableHead>
              <TableHead>
                <Trans>Email</Trans>
              </TableHead>
              <TableHead>
                <Trans>Phone</Trans>
              </TableHead>
              <TableHead className="text-right">
                <Trans>Actions</Trans>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {query.isPending &&
              Array.from({ length: 5 }, (_, i) => (
                <TableRow key={i} aria-hidden>
                  <TableCell>
                    <Skeleton className="h-4 w-32" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-40" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-28" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="ml-auto h-4 w-12" />
                  </TableCell>
                </TableRow>
              ))}
            {query.isError && (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-destructive">
                  <Trans>Could not load customers.</Trans>
                </TableCell>
              </TableRow>
            )}
            {query.isSuccess && customers.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                  <Trans>No customers found.</Trans>
                </TableCell>
              </TableRow>
            )}
            {customers.map((c) => (
              <TableRow
                key={c.id}
                className="cursor-pointer"
                role="link"
                tabIndex={0}
                aria-label={t`Open customer`}
                onClick={(e) => {
                  if (isInteractiveTarget(e.target, e.currentTarget)) return
                  navigate({ to: "/customers/$customerId", params: { customerId: c.id } })
                }}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return
                  e.preventDefault()
                  navigate({ to: "/customers/$customerId", params: { customerId: c.id } })
                }}
              >
                <TableCell className="font-medium">
                  {c.name}
                  {c.archivedAt && (
                    <Badge variant="secondary" className="ml-2">
                      <Trans>Archived</Trans>
                    </Badge>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">{c.company || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{c.email || "—"}</TableCell>
                <TableCell className="text-muted-foreground">{c.phone || "—"}</TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => setEditing(c)}
                      aria-label={t`Edit`}
                    >
                      <PencilIcon />
                    </Button>
                    {!c.archivedAt && (
                      <Button
                        size="icon-sm"
                        variant="ghost"
                        onClick={() => setArchiving(c)}
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
      </section>

      <footer className="flex items-center justify-between text-sm text-muted-foreground">
        <span>
          <Trans>
            Page {page} of {totalPages} · {total} total
          </Trans>
        </span>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => goToPage(page - 1)}
          >
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

      <CustomerFormDialog open={createOpen} onOpenChange={setCreateOpen} />
      <CustomerFormDialog
        open={editing !== undefined}
        onOpenChange={(open) => !open && setEditing(undefined)}
        customer={editing}
      />
      {archiving && (
        <ArchiveCustomerDialog
          open={archiving !== undefined}
          onOpenChange={(open) => !open && setArchiving(undefined)}
          customer={archiving}
        />
      )}
    </>
  )
}
