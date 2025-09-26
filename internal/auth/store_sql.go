package auth

import (
	"context"
	"database/sql"
)

type SQLStore struct {
	DB *sql.DB
}

func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{DB: db} }

func (s *SQLStore) CreateUser(email, username, passwordHash string) (User, error) {
	const q = `
		INSERT INTO public.users (email, username, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, username, password_hash,
		         COALESCE(token_version,1) AS token_version,
		         status, email_verified_at, created_at, updated_at, role;
	`
	var u User
	err := s.DB.QueryRowContext(context.Background(), q, email, username, passwordHash).Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.TokenVersion,
		&u.Status, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt, &u.Role,
	)
	return u, err
}

func (s *SQLStore) FindUserByEmail(email string) (User, error) {
	const q = `
		SELECT id, email, username, password_hash,
		       COALESCE(token_version,1) AS token_version,
		       status, email_verified_at, created_at, updated_at, role
		FROM public.users
		WHERE email = $1
		LIMIT 1;
	`
	var u User
	err := s.DB.QueryRowContext(context.Background(), q, email).Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.TokenVersion,
		&u.Status, &u.EmailVerified, &u.CreatedAt, &u.UpdatedAt, &u.Role,
	)
	return u, err
}

func (s *SQLStore) UpdateUserPasswordHash(userID, newHash string) error {
	const q = `UPDATE public.users SET password_hash = $1, updated_at = now() WHERE id = $2;`
	_, err := s.DB.ExecContext(context.Background(), q, newHash, userID)
	return err
}
