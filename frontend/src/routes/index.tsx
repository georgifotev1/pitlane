import { useQuery } from "@tanstack/react-query"
import { createFileRoute, useNavigate } from "@tanstack/react-router"
import { useEffect } from "react"
import { Trans } from "@lingui/react/macro"
import { api, ProblemError } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"

export const Route = createFileRoute("/")({
  component: Index,
})

// The root route is a dispatch, not a page: signed-in users land on the
// dashboard, everyone else on the login screen. The healthz probe that lived
// here in Phase 0 did its job — the shell is the home now.
function Index() {
  const navigate = useNavigate()
  const me = useQuery({
    queryKey: queryKeys.auth.me(),
    queryFn: api.me,
    retry: false,
  })

  useEffect(() => {
    if (me.isSuccess) {
      navigate({ to: "/dashboard" })
    } else if (me.isError && me.error instanceof ProblemError && me.error.status === 401) {
      navigate({ to: "/login" })
    }
  }, [me.isSuccess, me.isError, me.error, navigate])

  return (
    <main className="flex min-h-svh items-center justify-center text-sm text-muted-foreground">
      <Trans>Loading…</Trans>
    </main>
  )
}
