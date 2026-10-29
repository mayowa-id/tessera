package staff

import (
	"context"
	"errors"
)

var (
	ErrNotFound  = errors.New("record not found")
	ErrRole      = errors.New("job roles must belong to this organization and be active for new assignments")
	ErrNameInUse = errors.New("name is already in use")
)

type Employee struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Active      bool     `json:"active"`
	JobRoleIDs  []string `json:"job_role_ids"`
}
type CatalogItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}
type EmployeeInput struct {
	DisplayName string
	Active      bool
	JobRoleIDs  []string
}
type CatalogInput struct {
	Name   string
	Active bool
}
type Page struct {
	Offset, Limit   int32
	Search          string
	IncludeInactive bool
}

type Store interface {
	Employees(context.Context, string, Page) ([]Employee, error)
	Employee(context.Context, string, string) (Employee, error)
	SaveEmployee(context.Context, string, string, EmployeeInput) (Employee, error)
	Catalog(context.Context, string, string, Page) ([]CatalogItem, error)
	CatalogItem(context.Context, string, string, string) (CatalogItem, error)
	SaveCatalogItem(context.Context, string, string, string, CatalogInput) (CatalogItem, error)
}
