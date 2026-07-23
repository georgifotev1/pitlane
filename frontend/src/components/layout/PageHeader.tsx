type Props = {
  title: React.ReactNode
  description?: React.ReactNode
  actions?: React.ReactNode
}

// PageHeader is the one-page-introduction pattern: bold title, one-line
// description, and the page's primary actions pinned to the right. Every
// screen uses it, so the app reads consistently top to bottom.
export function PageHeader({ title, description, actions }: Props) {
  return (
    <header className="flex flex-wrap items-center justify-between gap-4">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </header>
  )
}
