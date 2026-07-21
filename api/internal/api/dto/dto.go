// Package dto defines the API boundary types. Only these types serialize to
// JSON; tygo generates the frontend's TypeScript types from them.
package dto

type HealthResponse struct {
	Status      string `json:"status"`
	Environment string `json:"environment"`
}

type SignupRequest struct {
	TenantName string `json:"tenantName"`
	UserName   string `json:"userName"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenantId"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

type SignupResponse struct {
	User UserResponse `json:"user"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}
