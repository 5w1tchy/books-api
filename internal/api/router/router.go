package router

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
	"github.com/5w1tchy/books-api/internal/api/handlers/foryou"
	"github.com/5w1tchy/books-api/internal/api/handlers/search"
	"github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/5w1tchy/books-api/internal/auth"
	"github.com/redis/go-redis/v9"
)

func Router(db *sql.DB, rdb *redis.Client) http.Handler {
	mux := http.NewServeMux()

	// Root & health
	mux.HandleFunc("GET /", handlers.RootHandler)
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.HandleFunc("HEAD /healthz", handlers.Healthz)

	// Books (reads open)
	mux.Handle("GET /books", books.Handler(db, rdb))        // list
	mux.Handle("GET /books/{key}", books.Handler(db, rdb))  // get
	mux.Handle("HEAD /books/{key}", books.Handler(db, rdb)) // head
	mux.Handle("OPTIONS /books", books.Handler(db, rdb))    // preflight
	mux.Handle("OPTIONS /books/{key}", books.Handler(db, rdb))

	// Books (writes) — admin only
	mux.Handle("POST /books", middlewares.RequireRole(db, "admin", books.Handler(db, rdb)))
	mux.Handle("PATCH /books/{key}", middlewares.RequireRole(db, "admin", books.Handler(db, rdb)))
	mux.Handle("PUT /books/{key}", middlewares.RequireRole(db, "admin", books.Handler(db, rdb)))
	mux.Handle("DELETE /books/{key}", middlewares.RequireRole(db, "admin", books.Handler(db, rdb)))

	// Search
	mux.Handle("GET /search/suggest", search.Suggest(db))

	// For-You feed
	feed := foryou.Handler(db, rdb)
	mux.Handle("GET /for-you", feed)
	mux.Handle("GET /for-you/", feed)

	// Auth
	authStore := auth.NewSQLStore(db)
	authH := auth.New(authStore, rdb)

	mux.HandleFunc("POST /auth/register", authH.Register)
	mux.Handle("POST /auth/login", middlewares.LoginRateLimit(rdb, http.HandlerFunc(authH.Login)))
	mux.HandleFunc("POST /auth/refresh", authH.Refresh)
	mux.HandleFunc("POST /auth/logout", authH.Logout)

	// Protected auth endpoints
	mux.Handle("GET /auth/me", middlewares.RequireAuth(db, http.HandlerFunc(authH.Me)))
	mux.Handle("POST /auth/logout-all", middlewares.RequireAuth(db, http.HandlerFunc(authH.LogoutAll)))
	mux.Handle("POST /auth/change-password", middlewares.RequireAuth(db, http.HandlerFunc(authH.ChangePassword)))

	// Email verification
	verify := &auth.VerifyDeps{DB: db, RDB: rdb, BaseURL: ""}
	mux.Handle("POST /auth/send-verification",
		middlewares.RequireAuth(db, verify.HandleSendVerification(
			func(r *http.Request) (string, bool) { return middlewares.UserIDFrom(r.Context()) },
		)),
	)
	mux.HandleFunc("GET /auth/verify", verify.HandleVerify())

	return mux
}
