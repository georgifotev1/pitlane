import { useForm } from "react-hook-form";
import { Link, createFileRoute } from "@tanstack/react-router";
import { useMutation } from "@tanstack/react-query";
import { Trans, useLingui } from "@lingui/react/macro";
import { api } from "@/lib/api";
import { applyServerErrors } from "@/lib/formErrors.ts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AuthLayout } from "@/components/layout/AuthLayout";

type ResetSearch = {
  token: string;
};

export const Route = createFileRoute("/reset-password")({
  // The token rides in the emailed link's query string.
  validateSearch: (search: Record<string, unknown>): ResetSearch => ({
    token: typeof search.token === "string" ? search.token : "",
  }),
  component: ResetPasswordPage,
});

type ResetForm = {
  password: string;
  confirm: string;
};

function ResetPasswordPage() {
  const { t } = useLingui();
  const { token } = Route.useSearch();
  const {
    register,
    handleSubmit,
    setError,
    getValues,
    formState: { errors, isSubmitting },
  } = useForm<ResetForm>({ defaultValues: { password: "", confirm: "" } });

  const reset = useMutation({
    mutationFn: (values: ResetForm) =>
      api.confirmPasswordReset({ token, password: values.password }),
    onError: (err) => applyServerErrors(setError, err),
  });

  // A link with no token is unusable — say so instead of showing a dead form.
  if (!token) {
    return (
      <AuthLayout
        title={<Trans>Invalid link</Trans>}
        description={
          <Trans>
            This reset link is missing its token. Please request a new one.
          </Trans>
        }
        footer={
          <Link
            to="/forgot-password"
            className="font-medium text-foreground underline-offset-4 hover:underline"
          >
            <Trans>Request a new link</Trans>
          </Link>
        }
      />
    );
  }

  if (reset.isSuccess) {
    return (
      <AuthLayout
        title={<Trans>Password updated</Trans>}
        description={
          <Trans>
            Your password has been changed. Sign in with your new password.
          </Trans>
        }
      >
        <Button className="w-full" render={<Link to="/login" />}>
          <Trans>Sign in</Trans>
        </Button>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout
      title={<Trans>Choose a new password</Trans>}
      description={
        <Trans>
          Signing in on other devices will require the new password.
        </Trans>
      }
    >
      <form
        className="space-y-4"
        onSubmit={handleSubmit((values) => reset.mutate(values))}
      >
        <div className="space-y-2">
          <Label htmlFor="password">
            <Trans>New password</Trans>
          </Label>
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            {...register("password", {
              required: t`This field is required.`,
              minLength: {
                value: 8,
                message: t`Must be at least 8 characters.`,
              },
            })}
          />
          {errors.password && (
            <p className="text-sm text-destructive">
              {errors.password.message}
            </p>
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
              validate: (v) =>
                v === getValues("password") || t`Passwords do not match.`,
            })}
          />
          {errors.confirm && (
            <p className="text-sm text-destructive">{errors.confirm.message}</p>
          )}
        </div>
        {errors.root && (
          <p className="text-sm text-destructive">{errors.root.message}</p>
        )}
        <Button
          type="submit"
          className="w-full"
          disabled={isSubmitting || reset.isPending}
        >
          {reset.isPending ? (
            <Trans>Saving…</Trans>
          ) : (
            <Trans>Set new password</Trans>
          )}
        </Button>
      </form>
    </AuthLayout>
  );
}
