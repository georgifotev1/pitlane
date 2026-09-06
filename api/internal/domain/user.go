package domain

import "time"

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleMechanic Role = "mechanic"
)

type Permission string

const (
	PermissionCustomersRead    Permission = "customers:read"
	PermissionCustomersWrite   Permission = "customers:write"
	PermissionCarsRead         Permission = "cars:read"
	PermissionCarsWrite        Permission = "cars:write"
	PermissionOffersRead       Permission = "offers:read"
	PermissionOffersWrite      Permission = "offers:write"
	PermissionRepairsRead      Permission = "repairs:read"
	PermissionRepairsWrite     Permission = "repairs:write"
	PermissionHistoryRead      Permission = "history:read"
	PermissionHistoryWrite     Permission = "history:write"
	PermissionAttachmentsRead  Permission = "attachments:read"
	PermissionAttachmentsWrite Permission = "attachments:write"
	PermissionUsersRead        Permission = "users:read"
	PermissionUsersWrite       Permission = "users:write"
	PermissionSettingsWrite    Permission = "settings:write"
)

var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
		PermissionUsersRead, PermissionUsersWrite,
		PermissionSettingsWrite,
	},
	RoleAdmin: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
		PermissionUsersRead, PermissionUsersWrite,
	},
	RoleMechanic: {
		PermissionCustomersRead,
		PermissionCarsRead,
		PermissionOffersRead,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
	},
}

func HasPermission(role Role, permission Permission) bool {
	for _, allowed := range RolePermissions[role] {
		if allowed == permission {
			return true
		}
	}
	return false
}

func PermissionsFor(role Role) []Permission {
	return RolePermissions[role]
}

func IsValidRole(value string) bool {
	switch Role(value) {
	case RoleOwner, RoleAdmin, RoleMechanic:
		return true
	}
	return false
}

type User struct {
	ID                string
	TenantID          string
	Email             string
	PasswordHash      string
	Role              Role
	Name              string
	PasswordChangedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
