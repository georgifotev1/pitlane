import { useNavigate } from "@tanstack/react-router"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Trans } from "@lingui/react/macro"
import { ChevronsUpDownIcon, LogOutIcon } from "lucide-react"
import type { UserResponse } from "@/lib/generated/types"
import { api } from "@/lib/api"
import { queryKeys } from "@/lib/queryKeys"
import { useRoleLabel } from "@/lib/roles"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"

// Initials for the avatar fallback: first letters of the first two words of
// the display name, uppercased. Falls back to the email's first letter.
function initialsOf(user: UserResponse): string {
  const words = user.name.trim().split(/\s+/).filter(Boolean)
  if (words.length === 0) return user.email.slice(0, 1).toUpperCase()
  return words
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase()
}

export function AppUserMenu({ user }: { user: UserResponse }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const roleLabel = useRoleLabel()

  const logout = useMutation({
    mutationFn: () => api.logout(),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      await navigate({ to: "/login" })
    },
  })

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={<SidebarMenuButton size="lg" className="cursor-pointer" />}
          >
            <Avatar className="size-8">
              <AvatarFallback>{initialsOf(user)}</AvatarFallback>
            </Avatar>
            <span className="grid flex-1 text-left text-sm leading-tight">
              <span className="truncate font-medium">{user.name}</span>
              <span className="truncate text-xs text-sidebar-foreground/60">{user.email}</span>
            </span>
            <ChevronsUpDownIcon className="ml-auto size-4 shrink-0 text-sidebar-foreground/60" />
          </DropdownMenuTrigger>
          <DropdownMenuContent side="top" align="start" className="w-56">
            <DropdownMenuLabel>
              <span className="text-muted-foreground">
                <Trans>Signed in as</Trans>
              </span>{" "}
              {roleLabel(user.role)}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              disabled={logout.isPending}
              onClick={() => logout.mutate()}
            >
              <LogOutIcon />
              <Trans>Sign out</Trans>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
