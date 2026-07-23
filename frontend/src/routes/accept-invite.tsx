import { useForm } from "react-hook-form"
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { applyServerErrors } from "@/lib/formErrors.ts"
import { queryKeys } from "@/lib/queryKeys"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AuthLayout } from "@/components/layout/AuthLayout"

type InviteSearch = {
  token: string
}

export const Route = createFileRoute("/accept-invite")({
  validateSearch: (search: Record<string, unknown>): InviteSearch => ({
    token: typeof search.token === "string" ? search.token : "",
  }),
  component: AcceptInvitePage,
})

type InviteForm = {
  name: string
  password: string
  confirm: string
}

function AcceptInvitePage() {
  const { t } = useLingui()
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const {
    register,
    handleSubmit,
    setError,
    getValues,
    formState: { errors, isSubmitting },
  } = useForm<InviteForm>({ defaultValues: { name: "", password: "", confirm: "" } })

  const accept = useMutation({
    mutationFn: (values: InviteForm) =>
      api.acceptInvite({ token, name: values.name, password: values.password }),
    // Accepting starts a session (like signup), so refresh `me` and go in.
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      await navigate({ to: "/dashboard" })
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  if (!token) {
    return (
      <AuthLayout
        title={<Trans>Invalid invitation</Trans>}
        description={
          <Trans>This invitation link is missing its token. Ask for a fresh invite.</Trans>
        }
        footer={
          <Link
            to="/login"
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            <Trans>Back to sign in</Trans>
          </Link>
        }
      />
    )
  }

  return (
    <AuthLayout
      title={<Trans>Accept your invitation</Trans>}
      description={<Trans>Set up your name and password to join the garage.</Trans>}
    >
      <form className="space-y-4" onSubmit={handleSubmit((values) => accept.mutate(values))}>
        <div className="space-y-2">
          <Label htmlFor="name">
            <Trans>Your name</Trans>
          </Label>
          <Input
            id="name"
            autoComplete="name"
            {...register("name", { required: t`This field is required.` })}
          />
          {errors.name && <p className="text-sm text-destructive">{errors.name.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">
            <Trans>Password</Trans>
          </Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            {...register("password", {
              required: t`This field is required.`,
              minLength: { value: 8, message: t`Must be at least 8 characters.` },
            })}
          />
          {errors.password && (
            <p className="text-sm text-destructive">{errors.password.message}</p>
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="confirm">
            <Trans>Confirm password</Trans>
          </Label>
          <Input
            id="confirm"
            type="password"
            autoComplete="new-password"
            {...register("confirm", {
              required: t`This field is required.`,
              validate: (v) => v === getValues("password") || t`Passwords do not match.`,
            })}
          />
          {errors.confirm && <p className="text-sm text-destructive">{errors.confirm.message}</p>}
        </div>
        {errors.root && <p className="text-sm text-destructive">{errors.root.message}</p>}
        <Button type="submit" className="w-full" disabled={isSubmitting || accept.isPending}>
          {accept.isPending ? <Trans>Joining…</Trans> : <Trans>Join garage</Trans>}
        </Button>
      </form>
    </AuthLayout>
  )
}
