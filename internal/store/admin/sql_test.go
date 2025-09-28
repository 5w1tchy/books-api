package adminstore_test

import (
	"regexp"
	"testing"
	"time"

	admin "github.com/5w1tchy/books-api/internal/api/handlers/admin"
	adminstore "github.com/5w1tchy/books-api/internal/store/admin"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestCountUsers(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := adminstore.New(db)

	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE email_verified_at IS NOT NULL) FROM public.users`,
	)).WillReturnRows(
		sqlmock.NewRows([]string{"count", "count"}).AddRow(42, 30),
	)

	total, verified, err := store.CountUsers(t.Context())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if total != 42 || verified != 30 {
		t.Fatalf("want total=42 verified=30; got total=%d verified=%d", total, verified)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetUserRole_OK(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := adminstore.New(db)

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE public.users SET role = $1 WHERE id = $2`,
	)).
		WithArgs("admin", "u-123").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetUserRole(t.Context(), "u-123", "admin"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetUserRole_NotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := adminstore.New(db)

	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE public.users SET role = $1 WHERE id = $2`,
	)).
		WithArgs("user", "nope").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected

	err = store.SetUserRole(t.Context(), "nope", "user")
	if err == nil {
		t.Fatalf("expected error for 0 rows affected")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListUsers_Basic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := adminstore.New(db)

	// COUNT(*) without WHERE
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT COUNT(*) FROM public.users`,
	)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// Prepare time.Time values
	t1, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	t2, _ := time.Parse(time.RFC3339, "2024-01-02T00:00:00Z")

	// SELECT page with LIMIT/OFFSET
	selectRe := regexp.MustCompile(
		`SELECT id::text, email, COALESCE\(username,''\), role, status, email_verified_at, created_at\s+` +
			`FROM public\.users\s+ORDER BY created_at DESC\s+LIMIT \$1 OFFSET \$2`,
	)

	rows := sqlmock.NewRows([]string{
		"id", "email", "username", "role", "status", "email_verified_at", "created_at",
	}).AddRow(
		"u1", "a@example.com", "alice", "user", "active", nil, t1,
	).AddRow(
		"u2", "b@example.com", "bob", "admin", "active", nil, t2,
	)

	mock.ExpectQuery(selectRe.String()).
		WithArgs(25, 0). // default Size=25 Page=1
		WillReturnRows(rows)

	list, total, err := store.ListUsers(t.Context(), admin.ListFilter{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("want total=2 items=2; got total=%d items=%d", total, len(list))
	}
	if list[0].ID != "u1" || list[1].ID != "u2" {
		t.Fatalf("unexpected order or data: %+v", list)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListUsers_WithFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	store := adminstore.New(db)
	f := admin.ListFilter{
		Query:    "ali",
		Role:     "admin",
		Status:   "active",
		Verified: ptr(false),
		Page:     2,
		Size:     10,
	}

	// COUNT with WHERE
	countRe := regexp.MustCompile(
		`SELECT COUNT\(\*\) FROM public\.users WHERE ` +
			`\((?i:email) ILIKE \$1 OR (?i:username) ILIKE \$1\) AND role = \$2 AND status = \$3 AND email_verified_at IS NULL`,
	)
	mock.ExpectQuery(countRe.String()).
		WithArgs("%ali%", "admin", "active").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(13))

	// SELECT with LIMIT/OFFSET
	tCreated, _ := time.Parse(time.RFC3339, "2024-02-02T00:00:00Z")
	selectRe := regexp.MustCompile(
		`SELECT id::text, email, COALESCE\(username,''\), role, status, email_verified_at, created_at\s+` +
			`FROM public\.users\s+WHERE ` +
			`\((?i:email) ILIKE \$1 OR (?i:username) ILIKE \$1\) AND role = \$2 AND status = \$3 AND email_verified_at IS NULL\s+` +
			`ORDER BY created_at DESC\s+LIMIT \$4 OFFSET \$5`,
	)

	rows := sqlmock.NewRows([]string{
		"id", "email", "username", "role", "status", "email_verified_at", "created_at",
	}).AddRow(
		"u9", "ali@example.com", "alice", "admin", "active", nil, tCreated,
	)

	// Page=2, Size=10 -> LIMIT 10 OFFSET 10
	mock.ExpectQuery(selectRe.String()).
		WithArgs("%ali%", "admin", "active", 10, 10).
		WillReturnRows(rows)

	items, total, err := store.ListUsers(t.Context(), f)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if total != 13 || len(items) != 1 || items[0].ID != "u9" {
		t.Fatalf("unexpected result: total=%d items=%d first=%+v", total, len(items), items[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func ptr(b bool) *bool { return &b }
