import { useState } from "react"
import { createFileRoute, Link } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import { ArrowRightIcon, PlusIcon, UsersIcon, WrenchIcon } from "lucide-react"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { formatMoney } from "@/lib/utils"
import { useRoleLabel } from "@/lib/roles"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { PageHeader } from "@/components/layout/PageHeader"
import { RepairStatusBadge } from "@/components/repairs/RepairStatusBadge"
import { CustomerFormDialog } from "@/components/customers/CustomerFormDialog"

export const Route = createFileRoute("/_authed/dashboard")({
  component: Dashboard,
})

// Tiny probes, big picture: pageSize 1 is enough to read metadata.total for
// the stat cards, and the two status-filtered lists power the active-repairs
// section. All of it rides endpoints that already exist (plan: zero backend
// changes).
function Dashboard() {
  const roleLabel = useRoleLabel()
  const [createOpen, setCreateOpen] = useState(false)

  const me = useQuery({ queryKey: queryKeys.auth.me(), queryFn: api.me })
  const customersProbe = useQuery({
    queryKey: queryKeys.customers.list({ page: 1, pageSize: 1, search: "", archived: false }),
    queryFn: () => api.customers.list({ page: 1, pageSize: 1, search: "", archived: false }),
  })
  const inProgress = useQuery({
    queryKey: queryKeys.repairs.list({ page: 1, pageSize: 4, status: "in_progress" }),
    queryFn: () => api.repairs.list({ page: 1, pageSize: 4, status: "in_progress" }),
  })
  const open = useQuery({
    queryKey: queryKeys.repairs.list({ page: 1, pageSize: 4, status: "open" }),
    queryFn: () => api.repairs.list({ page: 1, pageSize: 4, status: "open" }),
  })

  const activeTotal = (inProgress.data?.metadata.total ?? 0) + (open.data?.metadata.total ?? 0)
  const activeRepairs = [...(inProgress.data?.repairs ?? []), ...(open.data?.repairs ?? [])]
  const repairsPending = inProgress.isPending || open.isPending
  const repairsError = inProgress.isError || open.isError

  return (
    <>
      <PageHeader
        title={<Trans>Dashboard</Trans>}
        description={
          me.data ? (
            <>
              <Trans>Signed in as</Trans> {me.data.name} · {roleLabel(me.data.role)}
            </>
          ) : undefined
        }
        actions={
          <Button onClick={() => setCreateOpen(true)}>
            <PlusIcon />
            <Trans>New customer</Trans>
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <Card>
          <CardHeader>
            <CardDescription>
              <Trans>Customers</Trans>
            </CardDescription>
            <CardTitle className="text-3xl font-bold tabular-nums">
              {customersProbe.isPending ? (
                <Skeleton className="h-9 w-12" />
              ) : (
                (customersProbe.data?.metadata.total ?? 0)
              )}
            </CardTitle>
            <CardAction>
              <UsersIcon className="size-5 text-muted-foreground" />
            </CardAction>
          </CardHeader>
          <CardContent>
            <Button
              variant="link"
              className="px-0"
              nativeButton={false}
              render={
                <Link to="/customers" search={{ page: 1, search: "", archived: false }} />
              }
            >
              <Trans>All customers</Trans>
              <ArrowRightIcon />
            </Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardDescription>
              <Trans>Active repairs</Trans>
            </CardDescription>
            <CardTitle className="text-3xl font-bold tabular-nums">
              {repairsPending ? <Skeleton className="h-9 w-12" /> : activeTotal}
            </CardTitle>
            <CardAction>
              <WrenchIcon className="size-5 text-muted-foreground" />
            </CardAction>
          </CardHeader>
          <CardContent>
            <Button
              variant="link"
              className="px-0"
              nativeButton={false}
              render={<Link to="/repairs" search={{ page: 1, status: "" }} />}
            >
              <Trans>Repairs board</Trans>
              <ArrowRightIcon />
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>
            <Trans>Active repairs</Trans>
          </CardTitle>
          <CardDescription>
            <Trans>In progress and waiting to start, across the whole garage.</Trans>
          </CardDescription>
        </CardHeader>
        <CardContent>
          {repairsPending && (
            <div className="space-y-3" aria-hidden>
              <Skeleton className="h-6 w-full" />
              <Skeleton className="h-6 w-full" />
              <Skeleton className="h-6 w-2/3" />
            </div>
          )}
          {repairsError && (
            <p className="py-6 text-center text-sm text-destructive">
              <Trans>Could not load repairs.</Trans>
            </p>
          )}
          {!repairsPending && !repairsError && activeRepairs.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              <Trans>No active repairs right now.</Trans>
            </p>
          )}
          {activeRepairs.length > 0 && (
            <ul className="divide-y divide-border">
              {activeRepairs.map((r) => (
                <li key={r.id} className="flex items-center gap-4 py-3 first:pt-0 last:pb-0">
                  <Link
                    to="/repairs/$repairId"
                    params={{ repairId: r.id }}
                    className="font-medium underline-offset-4 hover:underline"
                  >
                    {r.carPlate}
                  </Link>
                  <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
                    {r.customerName}
                  </span>
                  <RepairStatusBadge status={r.status} />
                  <span className="w-24 text-right text-sm font-medium tabular-nums">
                    {formatMoney(r.totalCents)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <CustomerFormDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  )
}
