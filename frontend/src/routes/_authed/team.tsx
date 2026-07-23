import { useState } from "react"
import { createFileRoute } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"
import { Trans, useLingui } from "@lingui/react/macro"
import { PlusIcon, UserCogIcon, XIcon } from "lucide-react"
import type { InvitationResponse, UserResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { useRoleLabel } from "@/lib/roles"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { InviteUserDialog } from "@/components/team/InviteUserDialog"
import { ChangeRoleDialog } from "@/components/team/ChangeRoleDialog"
import { RevokeInviteDialog } from "@/components/team/RevokeInviteDialog"

export const Route = createFileRoute("/_authed/team")({
  component: TeamPage,
})

function TeamPage() {
  const { t } = useLingui()
  const roleLabel = useRoleLabel()

  const me = useQuery({ queryKey: queryKeys.auth.me(), queryFn: api.me })
  const canWrite = me.data?.permissions.includes("users:write") ?? false
  const canRead = me.data?.permissions.includes("users:read") ?? false

  const users = useQuery({
    queryKey: queryKeys.users.list(),
    queryFn: api.users.list,
    enabled: canRead,
  })
  const invitations = useQuery({
    queryKey: queryKeys.users.invitations(),
    queryFn: api.users.invitations.list,
    enabled: canRead,
  })

  const [inviteOpen, setInviteOpen] = useState(false)
  const [editingRole, setEditingRole] = useState<UserResponse | undefined>()
  const [revoking, setRevoking] = useState<InvitationResponse | undefined>()

  // Mechanics hold neither users:read nor users:write — the server 403s every
  // route, so the screen refuses rather than showing empty tables.
  if (me.isSuccess && !canRead) {
    return (
      <main className="mx-auto flex min-h-svh max-w-3xl flex-col justify-center gap-2 p-6 text-center">
        <h1 className="text-2xl font-bold tracking-tight">
          <Trans>Team</Trans>
        </h1>
        <p className="text-sm text-muted-foreground">
          <Trans>You don't have permission to manage the team.</Trans>
        </p>
      </main>
    )
  }

  const members = users.data?.users ?? []
  const pending = invitations.data?.invitations ?? []

  return (
    <main className="mx-auto flex min-h-svh max-w-4xl flex-col gap-8 p-6">
      <header className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">
            <Trans>Team</Trans>
          </h1>
          <p className="text-sm text-muted-foreground">
            <Trans>People with access to your garage.</Trans>
          </p>
        </div>
        {canWrite && (
          <Button onClick={() => setInviteOpen(true)}>
            <PlusIcon />
            <Trans>Invite teammate</Trans>
          </Button>
        )}
      </header>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">
          <Trans>Members</Trans>
        </h2>
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <Trans>Name</Trans>
                </TableHead>
                <TableHead>
                  <Trans>Email</Trans>
                </TableHead>
                <TableHead>
                  <Trans>Role</Trans>
                </TableHead>
                <TableHead className="text-right">
                  <Trans>Actions</Trans>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.isPending && (
                <TableRow>
                  <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                    <Trans>Loading…</Trans>
                  </TableCell>
                </TableRow>
              )}
              {users.isError && (
                <TableRow>
                  <TableCell colSpan={4} className="py-8 text-center text-destructive">
                    <Trans>Could not load the team.</Trans>
                  </TableCell>
                </TableRow>
              )}
              {members.map((u) => {
                // The owner's role is immutable and you can't change your own —
                // hide the control in both cases (the server also 409s).
                const editable = canWrite && u.role !== "owner" && u.id !== me.data?.id
                return (
                  <TableRow key={u.id}>
                    <TableCell className="font-medium">
                      {u.name}
                      {u.id === me.data?.id && (
                        <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                          <Trans>You</Trans>
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{u.email}</TableCell>
                    <TableCell>{roleLabel(u.role)}</TableCell>
                    <TableCell>
                      <div className="flex justify-end gap-1">
                        {editable && (
                          <Button
                            size="icon-sm"
                            variant="ghost"
                            onClick={() => setEditingRole(u)}
                            aria-label={t`Change role`}
                          >
                            <UserCogIcon />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </section>

      <section className="space-y-3">
        <h2 className="text-lg font-semibold">
          <Trans>Pending invitations</Trans>
        </h2>
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <Trans>Email</Trans>
                </TableHead>
                <TableHead>
                  <Trans>Role</Trans>
                </TableHead>
                <TableHead>
                  <Trans>Expires</Trans>
                </TableHead>
                <TableHead className="text-right">
                  <Trans>Actions</Trans>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {invitations.isSuccess && pending.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                    <Trans>No pending invitations.</Trans>
                  </TableCell>
                </TableRow>
              )}
              {pending.map((inv) => (
                <TableRow key={inv.id}>
                  <TableCell className="font-medium">{inv.email}</TableCell>
                  <TableCell>{roleLabel(inv.role)}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {new Date(inv.expiresAt).toLocaleDateString("bg-BG")}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-1">
                      {canWrite && (
                        <Button
                          size="icon-sm"
                          variant="ghost"
                          onClick={() => setRevoking(inv)}
                          aria-label={t`Revoke`}
                        >
                          <XIcon />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </section>

      <InviteUserDialog open={inviteOpen} onOpenChange={setInviteOpen} />
      {editingRole && (
        <ChangeRoleDialog
          open={editingRole !== undefined}
          onOpenChange={(open) => !open && setEditingRole(undefined)}
          user={editingRole}
        />
      )}
      {revoking && (
        <RevokeInviteDialog
          open={revoking !== undefined}
          onOpenChange={(open) => !open && setRevoking(undefined)}
          invitation={revoking}
        />
      )}
    </main>
  )
}
