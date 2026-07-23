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

export const Route = createFileRoute("/login")({
  component: LoginPage,
})

type LoginForm = {
  email: string
  password: string
}

function LoginPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({
    defaultValues: { email: "", password: "" },
  })

  const login = useMutation({
    mutationFn: (values: LoginForm) => api.login(values),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      const dest = sessionStorage.getItem("pitlane:redirect") || "/dashboard"
      sessionStorage.removeItem("pitlane:redirect")
      await navigate({ to: dest })
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  return (
    <main className="mx-auto flex min-h-svh max-w-sm flex-col justify-center gap-6 p-6">
      <div className="space-y-1 text-center">
        <h1 className="text-2xl font-bold tracking-tight">
          <Trans>Sign in to pitlane</Trans>
        </h1>
        <p className="text-sm text-muted-foreground">
          <Trans>Sign in with your garage credentials.</Trans>
        </p>
      </div>
      <form
        className="space-y-4"
        onSubmit={handleSubmit((values) => login.mutate(values))}
      >
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
        <div className="space-y-2">
          <Label htmlFor="password">
            <Trans>Password</Trans>
          </Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            {...register("password", { required: t`This field is required.` })}
          />
          {errors.password && <p className="text-sm text-destructive">{errors.password.message}</p>}
          <div className="text-right">
            <Link
              to="/forgot-password"
              className="text-sm text-muted-foreground underline-offset-4 hover:underline"
            >
              <Trans>Forgot your password?</Trans>
            </Link>
          </div>
        </div>
        {errors.root && <p className="text-sm text-destructive">{errors.root.message}</p>}
        <Button type="submit" className="w-full" disabled={isSubmitting || login.isPending}>
          {login.isPending ? <Trans>Signing in…</Trans> : <Trans>Sign in</Trans>}
        </Button>
      </form>
      <p className="text-center text-sm text-muted-foreground">
        <Trans>New here?</Trans>{" "}
        <Link to="/signup" className="font-medium text-foreground underline-offset-4 hover:underline">
          <Trans>Create your garage</Trans>
        </Link>
      </p>
    </main>
  )
}
