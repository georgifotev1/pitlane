import { ReactQueryDevtools } from "@tanstack/react-query-devtools"
import { createRootRoute, Link, Outlet } from "@tanstack/react-router"
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools"
import { Trans } from "@lingui/react/macro"
import { Button } from "@/components/ui/button"
import { TooltipProvider } from "@/components/ui/tooltip"

export const Route = createRootRoute({
  component: () => (
    <TooltipProvider>
      <div className="min-h-svh">
        <Outlet />
        {import.meta.env.DEV && (
          <>
            <TanStackRouterDevtools />
            <ReactQueryDevtools />
          </>
        )}
      </div>
    </TooltipProvider>
  ),
  notFoundComponent: NotFound,
})

function NotFound() {
  return (
    <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="text-6xl font-bold tracking-tight text-primary">404</p>
      <p className="text-muted-foreground">
        <Trans>This page does not exist.</Trans>
      </p>
      <Button render={<Link to="/dashboard" />}>
        <Trans>Back to the dashboard</Trans>
      </Button>
    </main>
  )
}
