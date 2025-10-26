package router

import (
	"database/sql"
	"net/http"

	"github.com/5w1tchy/books-api/internal/api/handlers"
	"github.com/5w1tchy/books-api/internal/api/handlers/books"
	"github.com/5w1tchy/books-api/internal/api/handlers/foryou"
	"github.com/5w1tchy/books-api/internal/api/handlers/search"
	"github.com/5w1tchy/books-api/internal/api/handlers/userbooks"
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

	// Books
	mux.Handle("GET /books", books.Handler(db, rdb))
	mux.Handle("OPTIONS /books", books.Handler(db, rdb))

	// Protected single-book view
	mux.Handle("GET /books/{key}", middlewares.RequireAuth(db, books.Get(db)))
	mux.Handle("HEAD /books/{key}", middlewares.RequireAuth(db, books.Head(db)))
	mux.Handle("OPTIONS /books/{key}", books.Handler(db, rdb))

	// --- Book audio streaming (presigned download) ---
	mux.Handle("GET /books/{key}/audio", books.GetBookAudioURLHandler(db))

	mux.Handle("GET /books/{key}/cover", books.GetBookCoverURLHandler(db))

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

	// User book features (require auth)
	mux.Handle("POST /user/reading-progress", middlewares.RequireAuth(db, userbooks.UpdateProgress(db)))
	mux.Handle("GET /user/reading-progress/{bookId}", middlewares.RequireAuth(db, userbooks.GetProgress(db)))
	mux.Handle("GET /user/continue-reading", middlewares.RequireAuth(db, userbooks.ContinueReading(db)))

	mux.Handle("POST /user/favorites/{bookId}", middlewares.RequireAuth(db, userbooks.AddFavorite(db)))
	mux.Handle("DELETE /user/favorites/{bookId}", middlewares.RequireAuth(db, userbooks.RemoveFavorite(db)))
	mux.Handle("GET /user/favorites", middlewares.RequireAuth(db, userbooks.GetFavorites(db)))

	mux.Handle("POST /user/books/{bookId}/notes", middlewares.RequireAuth(db, userbooks.AddNote(db)))
	mux.Handle("GET /user/books/{bookId}/notes", middlewares.RequireAuth(db, userbooks.GetNotes(db)))

	// Email verification
	verify := &auth.VerifyDeps{DB: db, RDB: rdb, BaseURL: ""}
	mux.Handle("POST /auth/send-verification",
		middlewares.RequireAuth(db, verify.HandleSendVerification(
			func(r *http.Request) (string, bool) { return middlewares.UserIDFrom(r.Context()) },
		)),
	)
	mux.HandleFunc("GET /auth/verify", verify.HandleVerify())

	// Admin (users, audit, stats, and admin-only book CRUD) — mounted via helper
	MountAdmin(mux, db, rdb)

	return mux
}
