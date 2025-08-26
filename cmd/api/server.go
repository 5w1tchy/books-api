package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/5w1tchy/books-api/internal/api/router"
	"github.com/5w1tchy/books-api/pkg/utils"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {

	_ = godotenv.Load("../../.env")

	port := ":3000"

	cert := "cert.pem"
	key := "key.pem"

	var rdb *redis.Client

	if url := os.Getenv("UPSTASH_REDIS_URL"); url != "" {
		// Path A: full Upstash URL (recommended)
		opt, err := redis.ParseURL(url) // e.g. rediss://default:<token>@host:port
		if err != nil {
			log.Fatalf("invalid UPSTASH_REDIS_URL: %v", err)
		}
		if opt.TLSConfig == nil {
			opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		opt.DialTimeout = 5 * time.Second
		opt.ReadTimeout = 1 * time.Second
		opt.WriteTimeout = 1 * time.Second
		rdb = redis.NewClient(opt)
	} else {
		// Path B: split fields
		addr := os.Getenv("REDIS_ADDR")     // host:port (no scheme)
		user := os.Getenv("REDIS_USER")     // "default" for Upstash
		pass := os.Getenv("REDIS_PASSWORD") // token
		if addr == "" || user == "" || pass == "" {
			log.Fatal("missing Redis config: set UPSTASH_REDIS_URL or REDIS_ADDR/REDIS_USER/REDIS_PASSWORD")
		}
		rdb = redis.NewClient(&redis.Options{
			Addr:         addr,
			Username:     user,
			Password:     pass,
			DB:           0,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  500 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
			TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
		})
	}

	// Fail fast if Redis isn’t reachable
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	fmt.Println("✅ Connected to Redis")

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
		mw.Cors,
		mw.ResponseTimeMiddleware,
		mw.HPP(hppOptions),
		tb.Middleware,
		sw.Middleware,
		mw.Compression,
		mw.SecurityHeaders,
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
