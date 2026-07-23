import { createFileRoute, Outlet, useNavigate } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { useEffect } from "react"
import { Trans, useLingui } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { ProblemError } from "@/lib/api"
import { AppSidebar } from "@/components/layout/AppSidebar"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { Separator } from "@/components/ui/separator"

export const Route = createFileRoute("/_authed")({
  component: AuthedLayout,
})

function AuthedLayout() {
  const { t } = useLingui()
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

  return (
    <SidebarProvider>
      <AppSidebar user={me.data} />
      <SidebarInset>
        <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
          <SidebarTrigger aria-label={t`Toggle sidebar`} />
          <Separator orientation="vertical" className="h-6" />
        </header>
        <div className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 p-6">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
