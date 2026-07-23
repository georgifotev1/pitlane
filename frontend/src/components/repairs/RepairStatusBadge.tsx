import { Trans } from "@lingui/react/macro"
import { Badge } from "@/components/ui/badge"

// Status color semantics, shared across the app (no new tokens):
//   open/pending  → secondary (neutral, nothing has happened yet)
//   in progress   → default   (primary orange, actively moving)
//   completed     → outline   (terminal, quiet)
//   failed states → destructive
export function RepairStatusBadge({ status }: { status: string }) {
  const variant =
    status === "in_progress" ? "default" : status === "completed" ? "outline" : "secondary"
  return (
    <Badge variant={variant}>
      {status === "open" && <Trans>Open</Trans>}
      {status === "in_progress" && <Trans>In progress</Trans>}
      {status === "completed" && <Trans>Completed</Trans>}
    </Badge>
  )
}
