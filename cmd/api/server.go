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
	// Load env when running from cmd/api (local dev)
	_ = godotenv.Load("../../.env")

	// -------- Redis (Upstash) ----------
	var rdb *redis.Client

	if url := os.Getenv("UPSTASH_REDIS_URL"); url != "" {
		opt, err := redis.ParseURL(url) // rediss://default:<token>@host:6379
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
		addr := os.Getenv("REDIS_ADDR")     // host:port
		user := os.Getenv("REDIS_USER")     // "default"
		pass := os.Getenv("REDIS_PASSWORD") // token
		if addr == "" || user == "" || pass == "" {
			log.Fatal("missing Redis config: set UPSTASH_REDIS_URL or REDIS_ADDR/REDIS_USER/REDIS_PASSWORD")
		}
		rdb = redis.NewClient(&redis.Options{
			Addr:         addr,
			Username:     user,
			Password:     pass,
			DB:           0,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  1 * time.Second,
			WriteTimeout: 1 * time.Second,
			TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
		})
	}

	// Fail fast if Redis isn't reachable
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	// -------- Rate limiting / middlewares ----------
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

	// -------- Serve: HTTP on Render, HTTPS locally ----------
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: secureMux,
	}

	// If PORT is set (Render), run plain HTTP (Render terminates TLS at the edge)
	if os.Getenv("PORT") != "" {
		fmt.Println("Server (Render) listening on port:", port, "(HTTP)")
		log.Fatal(server.ListenAndServe())
	}

	// Local dev: use mkcert certs for HTTPS
	cert := "cert.pem"
	key := "key.pem"
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	fmt.Println("Server (local) listening on port:", port, "(HTTPS)")
	log.Fatal(server.ListenAndServeTLS(cert, key))
}
