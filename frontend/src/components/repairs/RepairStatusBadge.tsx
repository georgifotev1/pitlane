import { Trans } from "@lingui/react/macro"

// RepairStatusBadge renders a repair's lifecycle state with a tone from the
// existing muted palette (no new tokens): primary for the terminal `completed`,
// a soft accent while in progress, neutral when still open.
export function RepairStatusBadge({ status }: { status: string }) {
  const tone =
    status === "completed"
      ? "bg-primary/10 text-primary"
      : status === "in_progress"
        ? "bg-accent text-accent-foreground"
        : "bg-muted text-muted-foreground"
  return (
    <span className={`rounded px-1.5 py-0.5 text-xs font-medium ${tone}`}>
      {status === "open" && <Trans>Open</Trans>}
      {status === "in_progress" && <Trans>In progress</Trans>}
      {status === "completed" && <Trans>Completed</Trans>}
    </span>
  )
}
