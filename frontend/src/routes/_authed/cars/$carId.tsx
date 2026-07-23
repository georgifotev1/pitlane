import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { Badge } from "@/components/ui/badge"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { OffersSection } from "@/components/offers/OffersSection"
import { HistorySection } from "@/components/history/HistorySection"
import { AttachmentsSection } from "@/components/attachments/AttachmentsSection"

export const Route = createFileRoute("/_authed/cars/$carId")({
  component: CarDetail,
})

function Field({ label, value }: { label: React.ReactNode; value: string }) {
  return (
    <div className="space-y-1">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="text-sm">{value || "—"}</dd>
    </div>
  )
}

function CarDetail() {
  const { t } = useLingui()
  const { carId } = Route.useParams()

  const query = useQuery({
    queryKey: queryKeys.cars.detail(carId),
    queryFn: () => api.cars.get(carId),
    retry: false,
  })

  // The car's customer supplies the default send-to email for offers. Dependent
  // on the car load (needs customerId); a miss just leaves the field empty.
  const customerId = query.data?.customerId
  const customerQuery = useQuery({
    queryKey: queryKeys.customers.detail(customerId ?? ""),
    queryFn: () => api.customers.get(customerId!),
    enabled: !!customerId,
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
          <Trans>Car not found.</Trans>
        </p>
      </div>
    )
  }

  const car = query.data

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
            <BreadcrumbLink
              render={
                <Link to="/customers/$customerId" params={{ customerId: car.customerId }} />
              }
            >
              {customerQuery.data?.name ?? "…"}
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{car.plate}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <header>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          {car.plate}
          {car.archivedAt && (
            <Badge variant="secondary">
              <Trans>Archived</Trans>
            </Badge>
          )}
        </h1>
        {(car.make || car.model) && (
          <p className="text-sm text-muted-foreground">
            {[car.make, car.model].filter(Boolean).join(" ")}
          </p>
        )}
      </header>

      <dl className="grid grid-cols-2 gap-6 rounded-lg border border-border p-6 sm:grid-cols-4">
        <Field label={t`Make`} value={car.make} />
        <Field label={t`Model`} value={car.model} />
        <Field label={t`Year`} value={car.year ? String(car.year) : ""} />
        <Field
          label={t`Mileage`}
          value={car.mileage ? car.mileage.toLocaleString("bg-BG") : ""}
        />
        <div className="col-span-2 sm:col-span-4">
          <Field label={t`VIN`} value={car.vin} />
        </div>
      </dl>

      <OffersSection carId={car.id} defaultRecipient={customerQuery.data?.email ?? ""} />

      <HistorySection carId={car.id} />

      <AttachmentsSection carId={car.id} />
    </>
  )
}
