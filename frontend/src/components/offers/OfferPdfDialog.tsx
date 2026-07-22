import { Trans, useLingui } from "@lingui/react/macro"
import { DownloadIcon } from "lucide-react"
import { api } from "@/lib/api"
import { cn } from "@/lib/utils"
import { buttonVariants } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  // The offer to preview. Undefined while the dialog is closed, so the iframe
  // is only mounted (and the PDF only fetched) when an offer is selected.
  offerId?: string
}

/**
 * Previews an offer's PDF in a same-origin iframe and offers a download link.
 * The iframe points straight at the API's inline-disposition endpoint; the
 * session cookie authenticates the request, so no blob juggling is needed
 * (ADR §13). The endpoint relaxes X-Frame-Options to SAMEORIGIN so this frames.
 */
export function OfferPdfDialog({ open, onOpenChange, offerId }: Props) {
  const { t } = useLingui()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            <Trans>Offer PDF</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>Preview the quote, then download or print it.</Trans>
          </DialogDescription>
        </DialogHeader>

        {offerId && (
          <iframe
            src={api.offers.pdfUrl(offerId, { inline: true })}
            title={t`Offer PDF preview`}
            className="h-[65vh] w-full rounded-lg border border-border bg-muted"
          />
        )}

        <DialogFooter>
          {offerId && (
            <a
              href={api.offers.pdfUrl(offerId)}
              download
              className={cn(buttonVariants({ variant: "default" }))}
            >
              <DownloadIcon />
              <Trans>Download</Trans>
            </a>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
