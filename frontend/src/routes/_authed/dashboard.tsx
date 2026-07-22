import { createFileRoute, Link, useNavigate } from "@tanstack/react-router"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { Button } from "@/components/ui/button"

export const Route = createFileRoute("/_authed/dashboard")({
  component: Dashboard,
})

function Dashboard() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const me = useQuery({ queryKey: queryKeys.auth.me(), queryFn: api.me })

  const logout = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      await navigate({ to: "/login" })
    },
  })

  if (me.isPending)
    return (
      <p className="p-6 text-sm text-muted-foreground">
        <Trans>Loading…</Trans>
      </p>
    )
  if (me.isError || !me.data) return null

  const user = me.data

  return (
    <main className="mx-auto flex min-h-svh max-w-3xl flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            <Trans>Dashboard</Trans>
          </h1>
          <p className="text-sm text-muted-foreground">
            <Trans>Signed in as</Trans> {user.name} · {user.email} · {user.role}
          </p>
        </div>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            logout.mutate()
          }}
        >
          <Button type="submit" variant="outline" disabled={logout.isPending}>
            <Trans>Sign out</Trans>
          </Button>
        </form>
      </header>
      <nav className="flex gap-2">
        <Link to="/customers" search={{ page: 1, search: "", archived: false }}>
          <Button variant="outline">
            <Trans>Customers</Trans>
          </Button>
        </Link>
        <Link to="/repairs" search={{ page: 1, status: "" }}>
          <Button variant="outline">
            <Trans>Repairs</Trans>
          </Button>
        </Link>
      </nav>
      <section className="rounded-lg border border-border p-6">
        <h2 className="text-lg font-semibold">
          <Trans>Permissions</Trans>
        </h2>
        <ul className="mt-2 grid grid-cols-2 gap-1 text-sm text-muted-foreground">
          {user.permissions.map((p) => (
            <li key={p} className="font-mono text-xs">
              {p}
            </li>
          ))}
        </ul>
      </section>
    </main>
  )
}
