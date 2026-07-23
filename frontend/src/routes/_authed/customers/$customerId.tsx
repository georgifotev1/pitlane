import { useState } from "react"
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PencilIcon, ArchiveIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { CustomerFormDialog } from "@/components/customers/CustomerFormDialog"
import { ArchiveCustomerDialog } from "@/components/customers/ArchiveCustomerDialog"
import { CarsSection } from "@/components/cars/CarsSection"

export const Route = createFileRoute("/_authed/customers/$customerId")({
  component: CustomerDetail,
})

function Field({ label, value }: { label: React.ReactNode; value: string }) {
  return (
    <div className="space-y-1">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="text-sm">{value || "—"}</dd>
    </div>
  )
}

function CustomerDetail() {
  const { t } = useLingui()
  const { customerId } = Route.useParams()
  const navigate = useNavigate()
  const [editOpen, setEditOpen] = useState(false)
  const [archiveOpen, setArchiveOpen] = useState(false)

  const query = useQuery({
    queryKey: queryKeys.customers.detail(customerId),
    queryFn: () => api.customers.get(customerId),
    retry: false,
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
      <div>
        <p className="text-sm text-destructive">
          <Trans>Customer not found.</Trans>
        </p>
        <Link
          to="/customers"
          search={{ page: 1, search: "", archived: false }}
          className="mt-4 inline-flex text-sm text-foreground underline-offset-4 hover:underline"
        >
          <Trans>Back to customers</Trans>
        </Link>
      </div>
    )
  }

  const c = query.data

  return (
    <>
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink
              render={
                <Link to="/customers" search={{ page: 1, search: "", archived: false }} />
              }
            >
              <Trans>Customers</Trans>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{c.name}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
            {c.name}
            {c.archivedAt && (
              <Badge variant="secondary">
                <Trans>Archived</Trans>
              </Badge>
            )}
          </h1>
          {c.company && <p className="text-sm text-muted-foreground">{c.company}</p>}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setEditOpen(true)}>
            <PencilIcon />
            <Trans>Edit</Trans>
          </Button>
          {!c.archivedAt && (
            <Button variant="outline" onClick={() => setArchiveOpen(true)}>
              <ArchiveIcon />
              <Trans>Archive</Trans>
            </Button>
          )}
        </div>
      </header>

      <dl className="grid grid-cols-1 gap-6 rounded-lg border border-border p-6 sm:grid-cols-2">
        <Field label={t`Email`} value={c.email} />
        <Field label={t`Phone`} value={c.phone} />
        <Field label={t`Company`} value={c.company} />
        <Field label={t`Address`} value={c.address} />
        <div className="sm:col-span-2">
          <Field label={t`Notes`} value={c.notes} />
        </div>
      </dl>

      <CarsSection customerId={c.id} />

      <CustomerFormDialog open={editOpen} onOpenChange={setEditOpen} customer={c} />
      <ArchiveCustomerDialog
        open={archiveOpen}
        onOpenChange={setArchiveOpen}
        customer={c}
        onArchived={() =>
          navigate({ to: "/customers", search: { page: 1, search: "", archived: false } })
        }
      />
    </>
  )
}
