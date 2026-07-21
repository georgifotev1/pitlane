// Package domain holds the business types and rules that are independent of
// the API boundary. DTOs serialize; these do not.
package domain

import "time"

// Role enumerates the fixed account roles. Permission grants are derived from
// RolePermissions below.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleMechanic Role = "mechanic"
)

// Permission enumerates the coarse actions the SPA can gate. Object-level
// checks (e.g. "does this user belong to this tenant?") live in the store or
// handler, not here.
type Permission string

const (
	PermissionCustomersRead  Permission = "customers:read"
	PermissionCustomersWrite Permission = "customers:write"
	PermissionOffersRead     Permission = "offers:read"
	PermissionOffersWrite    Permission = "offers:write"
	PermissionRepairsRead    Permission = "repairs:read"
	PermissionRepairsWrite   Permission = "repairs:write"
	PermissionUsersRead      Permission = "users:read"
	PermissionUsersWrite     Permission = "users:write"
	PermissionSettingsWrite  Permission = "settings:write"
)

// RolePermissions maps each role to the permissions it holds. The owner can do
// everything; admin and mechanic are subsets for the staff-invitation flow.
var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionUsersRead, PermissionUsersWrite,
		PermissionSettingsWrite,
	},
	RoleAdmin: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
	},
	RoleMechanic: {
		PermissionCustomersRead,
		PermissionOffersRead,
		PermissionRepairsRead, PermissionRepairsWrite,
	},
}

// HasPermission reports whether a role holds the given permission.
func HasPermission(role Role, p Permission) bool {
	for _, allowed := range RolePermissions[role] {
		if allowed == p {
			return true
		}
	}
	return false
}

// PermissionsFor returns the permission list for a role, or nil for unknown roles.
func PermissionsFor(role Role) []Permission {
	return RolePermissions[role]
}

// Tenant is the root entity that owns all data for one garage.
type Tenant struct {
	ID             string
	Name           string
	Address        string
	VATNumber      string
	LogoKey        string
	Currency       string
	Locale         string
	DefaultTaxRate int32
	Settings       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// User belongs to exactly one tenant. Email is globally unique.
type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Role         Role
	Name         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// IsValidRole reports whether a role string is one of the known roles.
func IsValidRole(s string) bool {
	switch Role(s) {
	case RoleOwner, RoleAdmin, RoleMechanic:
		return true
	}
	return false
}
