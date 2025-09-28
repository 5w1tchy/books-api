package adminstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	admin "github.com/5w1tchy/books-api/internal/api/handlers/admin"
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) admin.Store { return &Store{db: db} }

// ---------- helpers ----------

func buildListUsersQuery(f admin.ListFilter) (where string, args []any) {
	clauses := make([]string, 0, 4)
	if f.Query != "" {
		args = append(args, "%"+strings.TrimSpace(f.Query)+"%")
		clauses = append(clauses, fmt.Sprintf("(email ILIKE $%d OR username ILIKE $%d)", len(args), len(args)))
	}
	if f.Role != "" {
		args = append(args, f.Role)
		clauses = append(clauses, fmt.Sprintf("role = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Verified != nil {
		if *f.Verified {
			clauses = append(clauses, "email_verified_at IS NOT NULL")
		} else {
			clauses = append(clauses, "email_verified_at IS NULL")
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildAuditWhere(f admin.AuditFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if f.ActorID != "" {
		args = append(args, f.ActorID)
		clauses = append(clauses, fmt.Sprintf("admin_id = $%d", len(args)))
	}
	if f.TargetID != "" {
		args = append(args, f.TargetID)
		clauses = append(clauses, fmt.Sprintf("target_id = $%d", len(args)))
	}
	if f.Action != "" {
		args = append(args, f.Action)
		clauses = append(clauses, fmt.Sprintf("action = $%d", len(args)))
	}
	if f.Since != nil {
		args = append(args, *f.Since)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if f.Until != nil {
		args = append(args, *f.Until)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// ---------- methods ----------

func (s *Store) ListUsers(ctx context.Context, f admin.ListFilter) ([]admin.UserRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 || f.Size > 200 {
		f.Size = 25
	}
	where, args := buildListUsersQuery(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.users "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Size
	argsWithPage := append(append([]any{}, args...), f.Size, offset)
	listSQL := `
SELECT id::text, email, COALESCE(username,''), role, status, email_verified_at, created_at
FROM public.users
` + where + `
ORDER BY created_at DESC
LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := s.db.QueryContext(ctx, listSQL, argsWithPage...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]admin.UserRow, 0, f.Size)
	for rows.Next() {
		var u admin.UserRow
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &u.Status, &u.EmailVerified, &u.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*admin.UserRow, error) {
	const q = `
SELECT id::text, email, COALESCE(username,''), role, status, email_verified_at, created_at
FROM public.users
WHERE id = $1`
	var u admin.UserRow
	if err := s.db.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.Username, &u.Role, &u.Status, &u.EmailVerified, &u.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) SetUserStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE public.users SET status = $1 WHERE id = $2`
	res, err := s.db.ExecContext(ctx, q, status, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetUserRole(ctx context.Context, id, role string) error {
	const q = `UPDATE public.users SET role = $1 WHERE id = $2`
	res, err := s.db.ExecContext(ctx, q, role, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) BumpTokenVersion(ctx context.Context, id string) error {
	const q = `UPDATE public.users SET token_version = COALESCE(token_version,1) + 1 WHERE id = $1`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CountUsers(ctx context.Context) (int, int, error) {
	const q = `SELECT COUNT(*), COUNT(*) FILTER (WHERE email_verified_at IS NOT NULL) FROM public.users`
	var total, verified int
	if err := s.db.QueryRowContext(ctx, q).Scan(&total, &verified); err != nil {
		return 0, 0, err
	}
	return total, verified, nil
}

func (s *Store) CountBooks(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM public.books`
	var n int
	if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) CountSignupsLast24h(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM public.users WHERE created_at >= now() - interval '24 hours'`
	var n int
	if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM public.users WHERE role = 'admin'`
	var n int
	if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) InsertAudit(ctx context.Context, adminID, action, targetID string, meta any) error {
	var metaJSON string
	if meta == nil {
		metaJSON = "{}"
	} else {
		b, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		metaJSON = string(b)
	}
	const q = `
INSERT INTO public.admin_audit (admin_id, action, target_id, meta, created_at)
VALUES ($1, $2, $3, $4::jsonb, $5)`
	_, err := s.db.ExecContext(ctx, q, adminID, action, nullIfEmpty(targetID), metaJSON, time.Now().UTC())
	return err
}

func (s *Store) ListAudit(ctx context.Context, f admin.AuditFilter) ([]admin.AuditRow, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 || f.Size > 200 {
		f.Size = 25
	}

	where, args := buildAuditWhere(f)

	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.admin_audit "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (f.Page - 1) * f.Size
	argsWithPage := append(append([]any{}, args...), f.Size, offset)

	listSQL := `
SELECT id, admin_id::text, action, target_id::text, meta, created_at
FROM public.admin_audit
` + where + `
ORDER BY created_at DESC
LIMIT $` + fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)

	rows, err := s.db.QueryContext(ctx, listSQL, argsWithPage...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]admin.AuditRow, 0, f.Size)
	for rows.Next() {
		var row admin.AuditRow
		var tgt sql.NullString
		var metaRaw json.RawMessage
		if err := rows.Scan(&row.ID, &row.AdminID, &row.Action, &tgt, &metaRaw, &row.CreatedAt); err != nil {
			return nil, 0, err
		}
		if tgt.Valid {
			row.TargetID = &tgt.String
		}
		if len(metaRaw) == 0 {
			row.Meta = map[string]any{}
		} else {
			var anyMeta any
			if err := json.Unmarshal(metaRaw, &anyMeta); err == nil {
				row.Meta = anyMeta
			} else {
				row.Meta = string(metaRaw) // fallback
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
