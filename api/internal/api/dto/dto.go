// Package dto defines the API boundary types. Only these types serialize to
// JSON; tygo generates the frontend's TypeScript types from them.
package dto

type HealthResponse struct {
	Status      string `json:"status"`
	Environment string `json:"environment"`
}
