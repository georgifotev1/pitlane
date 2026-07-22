import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, TrashIcon, PencilIcon } from "lucide-react"
import { api, ProblemError } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { errorCodeToMessage } from "@/lib/errorCodes"
import { formatMoney } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"

interface HistorySectionProps {
  carId: string
}

export function HistorySection({ carId }: HistorySectionProps) {
  const { t } = useLingui()
  const qc = useQueryClient()
  const [formError, setFormError] = useState("")
  const [title, setTitle] = useState("")
  const [description, setDescription] = useState("")
  const [recordedAt, setRecordedAt] = useState(() => {
    const now = new Date()
    now.setMinutes(now.getMinutes() - now.getTimezoneOffset())
    return now.toISOString().slice(0, 16)
  })
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editTitle, setEditTitle] = useState("")
  const [editDescription, setEditDescription] = useState("")
  const [editRecordedAt, setEditRecordedAt] = useState("")

  const query = useQuery({
    queryKey: queryKeys.history.forCar(carId),
    queryFn: () => api.history.list(carId),
    retry: false,
  })

  const createMutation = useMutation({
    mutationFn: (data: { carId: string; title: string; description: string; recordedAt: string }) =>
      api.history.createNote(data.carId, data),
    onMutate: () => setFormError(""),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.history.all() })
      setTitle("")
      setDescription("")
    },
    onError: (err) => {
      const code = err instanceof ProblemError ? err.code : undefined
      setFormError(errorCodeToMessage(code) || t`The note could not be saved.`)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Parameters<typeof api.history.updateNote>[1] }) =>
      api.history.updateNote(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.history.all() })
      setEditingId(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: api.history.deleteNote,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.history.all() })
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    createMutation.mutate({
      carId,
      title,
      description,
      recordedAt: new Date(recordedAt).toISOString(),
    })
  }


  if (query.isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        <Trans>Loading history…</Trans>
      </p>
    )
  }

  if (query.isError) {
    return (
      <p className="text-sm text-destructive">
        <Trans>Could not load service history.</Trans>
      </p>
    )
  }

  const history = query.data?.history ?? []

  return (
    <section className="flex flex-col gap-6">
      <h2 className="text-lg font-semibold">
        <Trans>Service history</Trans>
      </h2>

      <form onSubmit={handleSubmit} className="flex flex-col gap-3 rounded-lg border border-border p-4">
        <div className="grid gap-2">
          <Label htmlFor="history-title">
            <Trans>Title</Trans>
          </Label>
          <Input
            id="history-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder={t`e.g. Dealer service`}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="history-description">
            <Trans>Description</Trans>
          </Label>
          <Textarea
            id="history-description"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t`What was done?`}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="history-date">
            <Trans>Date</Trans>
          </Label>
          <Input
            id="history-date"
            type="datetime-local"
            value={recordedAt}
            onChange={(e) => setRecordedAt(e.target.value)}
          />
        </div>
        {formError && <p className="text-sm text-destructive">{formError}</p>}
        <div className="flex justify-end">
          <Button type="submit" size="sm" disabled={createMutation.isPending}>
            <PlusIcon className="size-4" />
            <Trans>Add note</Trans>
          </Button>
        </div>
      </form>

      {history.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          <Trans>No history entries yet. Completed repairs and manual notes will appear here.</Trans>
        </p>
      ) : (
        <ol className="relative space-y-6 border-l border-border pl-6">
          {history.map((entry) => {
            const date = new Date(entry.recordedAt)
            const isEditing = editingId === entry.id
            return (
              <li key={entry.id} className="relative">
                <span className="absolute -left-[31px] top-1 flex size-4 items-center justify-center rounded-full border border-border bg-background" />
                <div className="space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-medium text-muted-foreground">
                      {date.toLocaleDateString("bg-BG")}
                    </span>
                    {entry.type === "note" && (
                      <div className="flex gap-1">
                        <Button
                          size="icon"
                          variant="ghost"
                          className="size-7"
                          onClick={() => {
                            setEditingId(entry.id)
                            setEditTitle(entry.title)
                            setEditDescription(entry.description)
                            const d = new Date(entry.recordedAt)
                            d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
                            setEditRecordedAt(d.toISOString().slice(0, 16))
                          }}
                        >
                          <PencilIcon className="size-4" />
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="size-7"
                          onClick={() => deleteMutation.mutate(entry.id)}
                          disabled={deleteMutation.isPending}
                        >
                          <TrashIcon className="size-4" />
                        </Button>
                      </div>
                    )}
                  </div>
                  {isEditing ? (
                    <form
                      className="flex flex-col gap-2"
                      onSubmit={(e) => {
                        e.preventDefault()
                        updateMutation.mutate({
                          id: entry.id,
                          data: {
                            title: editTitle,
                            description: editDescription,
                            recordedAt: new Date(editRecordedAt).toISOString(),
                          },
                        })
                      }}
                    >
                      <Input value={editTitle} onChange={(e) => setEditTitle(e.target.value)} />
                      <Textarea
                        value={editDescription}
                        onChange={(e) => setEditDescription(e.target.value)}
                      />
                      <Input
                        type="datetime-local"
                        value={editRecordedAt}
                        onChange={(e) => setEditRecordedAt(e.target.value)}
                      />
                      <div className="flex gap-2">
                        <Button type="submit" size="sm" disabled={updateMutation.isPending}>
                          <Trans>Save</Trans>
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditingId(null)}
                        >
                          <Trans>Cancel</Trans>
                        </Button>
                      </div>
                    </form>
                  ) : (
                    <>
                      <h3 className="text-sm font-semibold">
                        {entry.type === "repair" ? <Trans>Repair</Trans> : entry.title}
                      </h3>
                      {entry.description && (
                        <p className="whitespace-pre-line text-sm text-muted-foreground">
                          {entry.description}
                        </p>
                      )}
                      {entry.type === "repair" && (
                        <p className="text-sm text-muted-foreground">
                          <Trans>
                            Mileage {entry.mileage.toLocaleString("bg-BG")} km · Total{" "}
                            {formatMoney(entry.totalCents)}
                          </Trans>
                        </p>
                      )}
                    </>
                  )}
                </div>
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
