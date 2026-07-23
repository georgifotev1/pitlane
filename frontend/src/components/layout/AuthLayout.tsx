import { Link } from "@tanstack/react-router"
import { FlagIcon } from "lucide-react"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

type Props = {
  title: React.ReactNode
  description?: React.ReactNode
  // Confirmation states (e.g. "check your email") are title+description only.
  children?: React.ReactNode
  footer?: React.ReactNode
}

// AuthLayout frames every public form screen (login, signup, password reset,
// invite acceptance): muted backdrop, brand mark, one centered card. The
// brand uses the orange primary token — the accent users meet before they
// ever see the dark sidebar.
export function AuthLayout({ title, description, children, footer }: Props) {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-6 bg-muted/50 p-6">
      <Link to="/" className="flex items-center gap-2" aria-label="pitlane">
        <span className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <FlagIcon className="size-5" />
        </span>
        <span className="text-2xl font-bold tracking-tight">pitlane</span>
      </Link>
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-xl">{title}</CardTitle>
          {description ? <CardDescription>{description}</CardDescription> : null}
        </CardHeader>
        {children ? <CardContent>{children}</CardContent> : null}
      </Card>
      {footer ? <div className="text-center text-sm text-muted-foreground">{footer}</div> : null}
    </main>
  )
}
