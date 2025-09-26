package auth

import "time"

type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

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

// Keep DB details abstract so we don’t assume your SQL layer.
type UserStore interface {
	CreateUser(email, username, passwordHash string) (User, error)
	FindUserByEmail(email string) (User, error)
	UpdateUserPasswordHash(userID, newHash string) error
}
