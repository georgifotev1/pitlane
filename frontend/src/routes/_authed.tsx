import { createFileRoute, Outlet, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useEffect } from "react"
import { Trans } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { ProblemError } from "@/lib/api"

export const Route = createFileRoute("/_authed")({
  component: AuthedLayout,
})

function AuthedLayout() {
  const navigate = useNavigate()
  const me = useQuery({
    queryKey: queryKeys.auth.me(),
    queryFn: api.me,
    retry: false,
  })

  useEffect(() => {
    if (me.isError && me.error instanceof ProblemError && me.error.status === 401) {
      const here = window.location.pathname + window.location.search
      navigate({ to: "/login", search: { redirect: here } })
    }
  }, [me.isError, me.error, navigate])

  if (me.isPending) {
    return (
      <div className="flex min-h-svh items-center justify-center text-sm text-muted-foreground">
        <Trans>Loading…</Trans>
      </div>
    )
  }

  if (me.isError) {
    return null
  }

  return <Outlet />
}
