import { useForm } from "react-hook-form"
import { Link, createFileRoute } from "@tanstack/react-router"
import { useMutation } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { api } from "@/lib/api"
import { applyServerErrors } from "@/lib/formErrors.ts"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { AuthLayout } from "@/components/layout/AuthLayout"

export const Route = createFileRoute("/forgot-password")({
  component: ForgotPasswordPage,
})

type ForgotForm = {
  email: string
}

function ForgotPasswordPage() {
  const { t } = useLingui()
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ForgotForm>({ defaultValues: { email: "" } })

  const request = useMutation({
    mutationFn: (values: ForgotForm) => api.requestPasswordReset(values),
    // Success is enumeration-safe: the server returns the same 204 whether or
    // not the email exists, so we always show the same confirmation.
    onError: (err) => applyServerErrors(setError, err),
  })

  if (request.isSuccess) {
    return (
      <AuthLayout
        title={<Trans>Check your email</Trans>}
        description={
          <Trans>
            If an account exists for that address, we've sent a link to reset the password. The
            link is valid for one hour.
          </Trans>
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
      title={<Trans>Reset your password</Trans>}
      description={<Trans>Enter your email and we'll send you a reset link.</Trans>}
      footer={
        <Link
          to="/login"
          className="font-medium text-foreground underline-offset-4 hover:underline"
        >
          <Trans>Back to sign in</Trans>
        </Link>
      }
    >
      <form className="space-y-4" onSubmit={handleSubmit((values) => request.mutate(values))}>
        <div className="space-y-2">
          <Label htmlFor="email">
            <Trans>Email</Trans>
          </Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            {...register("email", { required: t`This field is required.` })}
          />
          {errors.email && <p className="text-sm text-destructive">{errors.email.message}</p>}
        </div>
        {errors.root && <p className="text-sm text-destructive">{errors.root.message}</p>}
        <Button type="submit" className="w-full" disabled={isSubmitting || request.isPending}>
          {request.isPending ? <Trans>Sending…</Trans> : <Trans>Send reset link</Trans>}
        </Button>
      </form>
    </AuthLayout>
  )
}
