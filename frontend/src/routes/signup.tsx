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

export const Route = createFileRoute("/signup")({
  component: SignupPage,
})

type SignupForm = {
  tenantName: string
  userName: string
  email: string
  password: string
}

function SignupPage() {
  const { t } = useLingui()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<SignupForm>({
    defaultValues: { tenantName: "", userName: "", email: "", password: "" },
  })

  const signup = useMutation({
    mutationFn: (values: SignupForm) => api.signup(values),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      await navigate({ to: "/dashboard" })
    },
    onError: (err) => applyServerErrors(setError, err),
  })

  const requiredMessage = t`This field is required.`
  const passwordMinMessage = t`Must be at least 8 characters.`

  return (
    <AuthLayout
      title={<Trans>Create your garage</Trans>}
      description={<Trans>Set up a garage and your owner account.</Trans>}
      footer={
        <p>
          <Trans>Already have an account?</Trans>{" "}
          <Link
            to="/login"
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            <Trans>Sign in</Trans>
          </Link>
        </p>
      }
    >
      <form
        className="space-y-4"
        onSubmit={handleSubmit((values) => signup.mutate(values))}
      >
        <div className="space-y-2">
          <Label htmlFor="tenantName">
            <Trans>Garage name</Trans>
          </Label>
          <Input
            id="tenantName"
            autoComplete="organization"
            {...register("tenantName", { required: requiredMessage })}
          />
          {errors.tenantName && <p className="text-sm text-destructive">{errors.tenantName.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="userName">
            <Trans>Your name</Trans>
          </Label>
          <Input
            id="userName"
            autoComplete="name"
            {...register("userName", { required: requiredMessage })}
          />
          {errors.userName && <p className="text-sm text-destructive">{errors.userName.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="email">
            <Trans>Email</Trans>
          </Label>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            {...register("email", { required: requiredMessage })}
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
            autoComplete="new-password"
            {...register("password", {
              required: requiredMessage,
              minLength: { value: 8, message: passwordMinMessage },
            })}
          />
          {errors.password && <p className="text-sm text-destructive">{errors.password.message}</p>}
        </div>
        {errors.root && <p className="text-sm text-destructive">{errors.root.message}</p>}
        <Button type="submit" className="w-full" disabled={isSubmitting || signup.isPending}>
          {signup.isPending ? <Trans>Creating…</Trans> : <Trans>Create garage</Trans>}
        </Button>
      </form>
    </AuthLayout>
  )
}
