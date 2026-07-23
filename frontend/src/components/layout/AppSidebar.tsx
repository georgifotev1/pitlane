import { Link, useRouterState } from "@tanstack/react-router"
import { Trans, useLingui } from "@lingui/react/macro"
import {
  CarIcon,
  FileTextIcon,
  FlagIcon,
  LayoutDashboardIcon,
  UsersIcon,
  WrenchIcon,
  UsersRoundIcon,
} from "lucide-react"
import type { UserResponse } from "@/lib/generated/types"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { AppUserMenu } from "@/components/layout/AppUserMenu"

type NavItem = {
  to: "/dashboard" | "/customers" | "/cars" | "/offers" | "/repairs" | "/team"
  label: string
  icon: React.ComponentType<{ className?: string }>
  // Predicate deciding whether the item is highlighted for the current path.
  // Section items stay highlighted on their detail pages (e.g. /customers/123
  // keeps "Customers" active) — the sidebar always answers "where am I".
  isActive: (pathname: string) => boolean
  // Permission code required to see the item; undefined = everyone.
  permission?: string
}

export function AppSidebar({ user }: { user: UserResponse }) {
  const { t } = useLingui()
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  const items: NavItem[] = [
    {
      to: "/dashboard",
      label: t`Dashboard`,
      icon: LayoutDashboardIcon,
      isActive: (p) => p === "/dashboard",
    },
    {
      to: "/customers",
      label: t`Customers`,
      icon: UsersIcon,
      isActive: (p) => p.startsWith("/customers"),
    },
    {
      to: "/cars",
      label: t`Cars`,
      icon: CarIcon,
      isActive: (p) => p.startsWith("/cars"),
    },
    {
      to: "/offers",
      label: t`Offers`,
      icon: FileTextIcon,
      isActive: (p) => p.startsWith("/offers"),
    },
    {
      to: "/repairs",
      label: t`Repairs`,
      icon: WrenchIcon,
      isActive: (p) => p.startsWith("/repairs"),
    },
    {
      to: "/team",
      label: t`Team`,
      icon: UsersRoundIcon,
      isActive: (p) => p === "/team",
      permission: "users:read",
    },
  ]
  const visible = items.filter(
    (item) => !item.permission || user.permissions.includes(item.permission)
  )

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/dashboard" />}>
              <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <FlagIcon className="size-4" />
              </span>
              <span className="text-lg font-bold tracking-tight">pitlane</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>
            <Trans>Menu</Trans>
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {visible.map((item) => (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    isActive={item.isActive(pathname)}
                    tooltip={item.label}
                    render={
                      item.to === "/customers" ? (
                        <Link to="/customers" search={{ page: 1, search: "", archived: false }} />
                      ) : item.to === "/cars" ? (
                        <Link to="/cars" search={{ page: 1, search: "", archived: false }} />
                      ) : item.to === "/offers" ? (
                        <Link to="/offers" search={{ page: 1, status: "" }} />
                      ) : item.to === "/repairs" ? (
                        <Link to="/repairs" search={{ page: 1, status: "" }} />
                      ) : (
                        <Link to={item.to} />
                      )
                    }
                  >
                    <item.icon />
                    <span>{item.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>
      <SidebarFooter>
        <AppUserMenu user={user} />
      </SidebarFooter>
    </Sidebar>
  )
}
