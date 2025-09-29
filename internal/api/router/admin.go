package router

import (
	"database/sql"
	"net/http"

	admin "github.com/5w1tchy/books-api/internal/api/handlers/admin"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
	"github.com/5w1tchy/books-api/internal/api/middlewares"
	adminstore "github.com/5w1tchy/books-api/internal/store/admin"
	"github.com/redis/go-redis/v9"
)

// MountAdmin wires all /admin/* endpoints behind RequireRole(..., "admin").
func MountAdmin(mux *http.ServeMux, db *sql.DB, rdb *redis.Client) {
	// Gate helper
	gate := func(next http.Handler) http.Handler {
		return middlewares.RequireRole(db, "admin", next)
	}

	// --- Admin handler (users, stats, audit) ---
	sto := adminstore.New(db)
	adminH := &admin.Handler{DB: db, RDB: rdb, Sto: sto}

	// Users management
	mux.Handle("GET /admin/users", gate(http.HandlerFunc(adminH.ListUsers)))
	mux.Handle("GET /admin/users/{id}", gate(http.HandlerFunc(adminH.GetUser)))
	mux.Handle("POST /admin/users/{id}/ban", gate(http.HandlerFunc(adminH.BanUser)))
	mux.Handle("POST /admin/users/{id}/unban", gate(http.HandlerFunc(adminH.UnbanUser)))
	mux.Handle("POST /admin/users/{id}/role", gate(http.HandlerFunc(adminH.SetRole)))
	mux.Handle("POST /admin/users/{id}/logout-all", gate(http.HandlerFunc(adminH.LogoutAll)))
	mux.Handle("POST /admin/users/{id}/resend-verification", gate(http.HandlerFunc(adminH.ResendVerification)))

	// Stats & audit
	mux.Handle("GET /admin/stats", gate(http.HandlerFunc(adminH.Stats)))
	mux.Handle("GET /admin/audit", gate(http.HandlerFunc(adminH.ListAudit)))

	// --- Admin-only Books CRUD ---
	mux.Handle("POST /admin/books", gate(books.AdminCreate(db, rdb)))
	mux.Handle("PATCH /admin/books/{key}", gate(books.AdminPatch(db, rdb)))
	mux.Handle("PUT /admin/books/{key}", gate(books.AdminPut(db, rdb)))
	mux.Handle("DELETE /admin/books/{key}", gate(books.AdminDelete(db, rdb)))
	mux.Handle("GET /admin/books", gate(books.AdminList(db, rdb)))
	mux.Handle("GET /admin/books/{key}", gate(books.AdminGet(db, rdb)))

	// --- Admin autocomplete endpoints ---
	mux.Handle("GET /admin/categories", gate(books.AdminGetCategories(db, rdb)))
	mux.Handle("GET /admin/authors", gate(books.AdminGetAuthors(db, rdb)))
}
