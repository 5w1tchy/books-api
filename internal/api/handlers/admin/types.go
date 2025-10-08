package admin

import (
	"context"
	"time"
)

// ===== DTOs =====

type UserRow struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	Role          string     `json:"role"`   // "user" | "admin"
	Status        string     `json:"status"` // "active" | "banned"
	EmailVerified *time.Time `json:"email_verified_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type AuditRow struct {
	ID        int64     `json:"id"`
	AdminID   string    `json:"admin_id"`
	Action    string    `json:"action"`
	TargetID  *string   `json:"target_id,omitempty"`
	Meta      any       `json:"meta"`
	CreatedAt time.Time `json:"created_at"`
}

// ===== Filters =====

type ListFilter struct {
	Query    string
	Role     string
	Verified *bool
	Status   string
	Page     int
	Size     int
}

type AuditFilter struct {
	ActorID  string
	TargetID string
	Action   string
	Since    *time.Time
	Until    *time.Time
	Page     int
	Size     int
}

// ===== Request Bodies =====

type SetRoleRequest struct {
	Role string `json:"role"`
}

type StatsResponse struct {
	UsersTotal     int `json:"users_total"`
	UsersVerified  int `json:"users_verified"`
	BooksTotal     int `json:"books_total"`
	SignupsLast24h int `json:"signups_last_24h"`
}

// ===== Store Interface =====

type Store interface {
	// Users
	ListUsers(ctx context.Context, filter ListFilter) ([]UserRow, int, error)
	GetUser(ctx context.Context, id string) (*UserRow, error)
	SetUserStatus(ctx context.Context, id, status string) error
	SetUserRole(ctx context.Context, id, role string) error
	BumpTokenVersion(ctx context.Context, id string) error
	CountUsers(ctx context.Context) (total, verified int, err error)
	CountBooks(ctx context.Context) (int, error)
	CountSignupsLast24h(ctx context.Context) (int, error)

	// Audit
	InsertAudit(ctx context.Context, adminID, action, targetID string, meta any) error
	ListAudit(ctx context.Context, f AuditFilter) ([]AuditRow, int, error)

	// Admins
	AdminCount(ctx context.Context) (int, error)
}
