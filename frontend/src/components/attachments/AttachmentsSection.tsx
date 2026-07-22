import { useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PaperclipIcon, DownloadIcon, TrashIcon, UploadIcon } from "lucide-react"
import { api, ProblemError } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { errorCodeToMessage } from "@/lib/errorCodes"
import { Button } from "@/components/ui/button"

interface AttachmentsSectionProps {
  carId?: string
  repairId?: string
}

const maxSizeMB = 10

export function AttachmentsSection({ carId, repairId }: AttachmentsSectionProps) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const inputRef = useRef<HTMLInputElement>(null)
  const [uploadError, setUploadError] = useState("")

  const listQuery = useQuery({
    queryKey: carId
      ? queryKeys.attachments.forCar(carId)
      : queryKeys.attachments.forRepair(repairId!),
    queryFn: () => (carId ? api.attachments.listByCar(carId) : api.repairs.attachments.list(repairId!)),
    retry: false,
  })

  const uploadMutation = useMutation({
    mutationFn: (file: File) => {
      if (carId) return api.attachments.uploadToCar(carId, file)
      return api.repairs.attachments.upload(repairId!, file)
    },
    onMutate: () => setUploadError(""),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.attachments.all() })
      if (inputRef.current) inputRef.current.value = ""
    },
    onError: (err) => {
      const code = err instanceof ProblemError ? err.code : undefined
      setUploadError(errorCodeToMessage(code) || t`The file could not be uploaded.`)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: api.attachments.delete,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.attachments.all() })
    },
  })

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    if (file.size > maxSizeMB * 1024 * 1024) {
      setUploadError(t`File is too large. Maximum size is 10 MB.`)
      return
    }
    uploadMutation.mutate(file)
  }

  const attachments = listQuery.data?.attachments ?? []

  return (
    <section className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">
          <Trans>Attachments</Trans>
        </h2>
        <Button
          size="sm"
          variant="outline"
          onClick={() => inputRef.current?.click()}
          disabled={uploadMutation.isPending}
        >
          <UploadIcon className="size-4" />
          <Trans>Upload</Trans>
        </Button>
        <input
          ref={inputRef}
          type="file"
          className="hidden"
          onChange={handleFileChange}
          accept="image/*,.pdf,.doc,.docx"
        />
      </div>

      {uploadError && <p className="text-sm text-destructive">{uploadError}</p>}

      {listQuery.isPending ? (
        <p className="text-sm text-muted-foreground">
          <Trans>Loading attachments…</Trans>
        </p>
      ) : listQuery.isError ? (
        <p className="text-sm text-destructive">
          <Trans>Could not load attachments.</Trans>
        </p>
      ) : attachments.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No attachments yet.</Trans>
        </p>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2">
          {attachments.map((att) => (
            <li
              key={att.id}
              className="flex items-center justify-between gap-3 rounded-lg border border-border p-3"
            >
              <div className="flex min-w-0 items-center gap-3">
                <PaperclipIcon className="size-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{att.name}</p>
                  <p className="text-xs text-muted-foreground">
                    {(att.sizeBytes / 1024).toFixed(1)} KB · {att.contentType}
                  </p>
                </div>
              </div>
              <div className="flex shrink-0 gap-1">
                <a
                  href={api.attachments.downloadUrl(att.id)}
                  download
                  className="inline-flex size-8 items-center justify-center rounded-md text-sm font-medium text-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                >
                  <DownloadIcon className="size-4" />
                </a>
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-8"
                  onClick={() => deleteMutation.mutate(att.id)}
                  disabled={deleteMutation.isPending}
                >
                  <TrashIcon className="size-4" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
