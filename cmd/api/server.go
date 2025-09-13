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
	"github.com/5w1tchy/books-api/internal/repository/sqlconnect"
	"github.com/5w1tchy/books-api/pkg/utils"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load("../../.env")

	db, err := sqlconnect.ConnectDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// -------- Redis ----------
	var rdb *redis.Client
	if url := os.Getenv("UPSTASH_REDIS_URL"); url != "" {
		opt, err := redis.ParseURL(url)
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
		addr := os.Getenv("REDIS_ADDR")
		user := os.Getenv("REDIS_USER")
		pass := os.Getenv("REDIS_PASSWORD")
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

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}

	// -------- Rate limiting: token-bucket only ----------
	tb := mw.NewRedisTokenBucket(rdb, 5, 20, mw.PerIPKey("tb"))

	hppOptions := mw.DefaultHPPOptions()

	secureMux := utils.ApplyMiddleware(
		router.Router(db, rdb),
		mw.RequestID,
		mw.Cors,
		mw.ResponseTimeMiddleware,
		mw.HPP(hppOptions),
		tb.Middleware,
		mw.Compression,
		mw.SecurityHeaders,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: secureMux,
	}

	if os.Getenv("PORT") != "" {
		fmt.Println("Server (Render) listening on port:", port, "(HTTP)")
		log.Fatal(server.ListenAndServe())
	}

	cert := "cert.pem"
	key := "key.pem"
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	fmt.Println("Server (local) listening on port:", port, "(HTTPS)")
	log.Fatal(server.ListenAndServeTLS(cert, key))
}

