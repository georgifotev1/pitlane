import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { ArrowLeftIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { OffersSection } from "@/components/offers/OffersSection"

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
      <main className="mx-auto max-w-3xl p-6">
        <p className="text-sm text-destructive">
          <Trans>Car not found.</Trans>
        </p>
      </main>
    )
  }

  const car = query.data

  return (
    <main className="mx-auto flex min-h-svh max-w-3xl flex-col gap-6 p-6">
      <Link
        to="/customers/$customerId"
        params={{ customerId: car.customerId }}
        className="inline-flex items-center gap-1 text-sm text-muted-foreground underline-offset-4 hover:underline"
      >
        <ArrowLeftIcon className="size-4" />
        <Trans>Back to customer</Trans>
      </Link>

      <header>
        <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
          {car.plate}
          {car.archivedAt && (
            <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-normal text-muted-foreground">
              <Trans>Archived</Trans>
            </span>
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
    </main>
  )
}
