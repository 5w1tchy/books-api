package auth

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// Handler holds dependencies for auth operations
type Handler struct {
	Store UserStore
	RDB   *redis.Client
}

// Request types

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// Response types

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type MeResponse struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Username      string     `json:"username"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	EmailVerified *time.Time `json:"email_verified_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// User model

type User struct {
	ID            string // uuid
	Email         string
	Username      string
	PasswordHash  string
	TokenVersion  int
	Status        string
	Role          string
	EmailVerified *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UserStore interface

type UserStore interface {
	CreateUser(email, username, passwordHash, role string) (User, error)
	FindUserByEmail(email string) (User, error)
	UpdateUserPasswordHash(userID, newHash string) error
}

// New creates a new auth handler instance
func New(store UserStore, rdb *redis.Client) *Handler {
	return &Handler{Store: store, RDB: rdb}
}
