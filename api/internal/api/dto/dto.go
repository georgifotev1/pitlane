// Package dto defines the API boundary types. Only these types serialize to
// JSON; tygo generates the frontend's TypeScript types from them.
package dto

import "time"

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

// ListMetadata is the pagination envelope carried alongside every list
// response: `{"customers": [...], "metadata": {...}}`. Reused by all list
// endpoints (ADR §8 offset/page pagination with metadata).
type ListMetadata struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

// CustomerResponse is the customer as seen by the client. ArchivedAt is null
// for active customers (tygo maps *time.Time → string | null).
type CustomerResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Company    string     `json:"company"`
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	Address    string     `json:"address"`
	Notes      string     `json:"notes"`
	ArchivedAt *time.Time `json:"archivedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// CustomerListResponse is the full body of the list endpoint. The client reads
// the whole object (both keys), unlike single-entity envelopes.
type CustomerListResponse struct {
	Customers []CustomerResponse `json:"customers"`
	Metadata  ListMetadata       `json:"metadata"`
}

// CreateCustomerRequest / UpdateCustomerRequest share a shape today, but stay
// distinct types so they can diverge without a breaking rename (Create may gain
// server-only defaults; Update is a full PUT replace).
type CreateCustomerRequest struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

type UpdateCustomerRequest struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

// CarResponse is a car as seen by the client. It belongs to a customer
// (CustomerID); ArchivedAt is null for active cars. year/mileage are 0 when
// unknown (tygo maps them to number).
type CarResponse struct {
	ID         string     `json:"id"`
	CustomerID string     `json:"customerId"`
	Plate      string     `json:"plate"`
	VIN        string     `json:"vin"`
	Make       string     `json:"make"`
	Model      string     `json:"model"`
	Year       int        `json:"year"`
	Mileage    int        `json:"mileage"`
	ArchivedAt *time.Time `json:"archivedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// CarListResponse is the full body of the list endpoint (both keys read by the
// client), mirroring CustomerListResponse.
type CarListResponse struct {
	Cars     []CarResponse `json:"cars"`
	Metadata ListMetadata  `json:"metadata"`
}

// CreateCarRequest / UpdateCarRequest stay distinct types even though they share
// a shape today (matching the customer pattern). CustomerID is not in the body:
// on create it comes from the nested route path; it is immutable on update.
type CreateCarRequest struct {
	Plate   string `json:"plate"`
	VIN     string `json:"vin"`
	Make    string `json:"make"`
	Model   string `json:"model"`
	Year    int    `json:"year"`
	Mileage int    `json:"mileage"`
}

type UpdateCarRequest struct {
	Plate   string `json:"plate"`
	VIN     string `json:"vin"`
	Make    string `json:"make"`
	Model   string `json:"model"`
	Year    int    `json:"year"`
	Mileage int    `json:"mileage"`
}
