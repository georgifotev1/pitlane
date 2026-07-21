import { useQuery } from "@tanstack/react-query"
import { createFileRoute } from "@tanstack/react-router"
import { Trans } from "@lingui/react/macro"
import { api } from "@/lib/api"

export const Route = createFileRoute("/")({
  component: Index,
})

function Index() {
  const health = useQuery({ queryKey: ["health"], queryFn: api.health })

  return (
    <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-2 p-6 text-center">
      <h1 className="text-3xl font-bold tracking-tight">pitlane</h1>
      {health.isPending && (
        <p className="text-muted-foreground">
          <Trans>Checking the API…</Trans>
        </p>
      )}
      {health.isError && (
        <p className="text-destructive">
          <Trans>API unreachable:</Trans> {health.error.message}
        </p>
      )}
      {health.isSuccess && (
        <p className="text-muted-foreground">
          API {health.data.status} · {health.data.environment}
        </p>
      )}
    </main>
  )
}
