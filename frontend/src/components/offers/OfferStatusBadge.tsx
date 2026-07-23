import { Trans } from "@lingui/react/macro"
import { Badge } from "@/components/ui/badge"

// OfferStatusBadge renders an offer's lifecycle status. Shared between the car
// detail page's offers section and the tenant-wide offers board so a status
// always looks the same everywhere (draft → neutral, sent → primary orange,
// accepted → quiet outline, rejected/expired → destructive).
export function OfferStatusBadge({ status }: { status: string }) {
  const variant =
    status === "sent"
      ? "default"
      : status === "accepted"
        ? "outline"
        : status === "rejected" || status === "expired"
          ? "destructive"
          : "secondary"
  return (
    <Badge variant={variant}>
      {status === "draft" && <Trans>Draft</Trans>}
      {status === "sent" && <Trans>Sent</Trans>}
      {status === "accepted" && <Trans>Accepted</Trans>}
      {status === "rejected" && <Trans>Rejected</Trans>}
      {status === "expired" && <Trans>Expired</Trans>}
    </Badge>
  )
}

// SendStatusBadge surfaces the email-delivery lifecycle (sendStatus) for offers
// that have been sent. It is meaningless on a draft, so nothing renders there.
export function SendStatusBadge({ status, sendStatus }: { status: string; sendStatus: string }) {
  if (status === "draft") return null
  const variant =
    sendStatus === "sent" ? "outline" : sendStatus === "failed" ? "destructive" : "secondary"
  return (
    <Badge variant={variant}>
      {sendStatus === "pending" && <Trans>Sending…</Trans>}
      {sendStatus === "sent" && <Trans>Emailed</Trans>}
      {sendStatus === "failed" && <Trans>Send failed</Trans>}
    </Badge>
  )
}
