package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"time"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/5w1tchy/books-api/internal/api/router"
	"github.com/5w1tchy/books-api/pkg/utils"
	"github.com/redis/go-redis/v9"
)

func main() {

	port := ":3000"

	cert := "cert.pem"
	key := "key.pem"

	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Change later to TLS 1.3
	}

	tb := mw.NewRedisTokenBucket(rdb, 5, 20, mw.PerIPKey("tb"))

	sw := mw.NewRedisSlidingWindow(rdb, 3000, 60*time.Minute, mw.PerIPKey("sw"))

	hppOptions := mw.HPPOptions{
		CheckQuery:                  true,
		CheckBody:                   true,
		CheckBodyOnlyForContentType: "application/x-www-form-urlencoded",
		Whitelist: []string{
			// General / shared
			"id", "user_id", "book_id", "chapter", "page", "limit", "offset",
			"lang", "search", "category", "tags",

			// Books
			"title", "author", "sort", "order",

			// Users
			"username", "email", "password", "token", "session_id",

			// Notes
			"note_id", "content", "created_at", "updated_at",

			// Highlights
			"highlight_id", "text", "color", "created_at",

			// Progress
			"progress_id", "percentage", "last_read_at",
		},
	}

	secureMux := utils.ApplyMiddleware(
		router.Router(),
		// Response shaping (inner)
		mw.Compression,     // compress final bytes
		mw.SecurityHeaders, // add security headers

		// Protections (request side)
		// NOTE: CORS before HPP so OPTIONS preflight isn’t blocked
		mw.HPP(hppOptions), // sanitize duplicate/unknown params
		tb.Middleware,      // token-bucket (coarse IP limiter)
		sw.Middleware,      // sliding-window (tighter IP limiter)

		// Cross-origin + timings (outer)
		mw.Cors,                   // handle preflights/headers early
		mw.ResponseTimeMiddleware, // measure full stack
	)
	// Create custom server
	server := &http.Server{
		Addr:    port,
		Handler: secureMux,
		// Handler:   mw.CORS(mux),
		TLSConfig: tlsConfig,
	}

	fmt.Println("Server is running on port:", port)
	err := server.ListenAndServeTLS(cert, key)
	if err != nil {
		log.Fatalln("Error starting server:", err)
	}
}
