package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	mw "github.com/5w1tchy/books-api/internal/api/middlewares"
	"github.com/5w1tchy/books-api/internal/api/router"
	"github.com/5w1tchy/books-api/internal/metrics/viewqueue"
	"github.com/5w1tchy/books-api/internal/repository/sqlconnect"
	validatePkg "github.com/5w1tchy/books-api/internal/validate"
	"github.com/5w1tchy/books-api/pkg/utils"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load("../../.env")

	// Validate critical env/config up front
	if err := validatePkg.Env(); err != nil {
		log.Fatal(err)
	}

	db, err := sqlconnect.ConnectDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	// --- start bounded view queue (2 workers, buffer 10k) ---
	viewqueue.Start(db, 10000, 2)
	defer viewqueue.Shutdown()
	// --------------------------------------------------------

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
	defer rdb.Close()

	// Ping Redis with timeout (replaces previous Background() ping)
	if err := validatePkg.PingRedis(rdb, 2*time.Second); err != nil {
		log.Fatalf("Redis connection failed: %v", err)
	}
	for _, w := range validatePkg.HardeningWarnings(os.Getenv("APP_ENV")) {
		log.Printf("WARN: %s", w)
	}

	// -------- Rate limiting: token-bucket only ----------
	tb := mw.NewRedisTokenBucket(rdb, 5, 20, mw.PerIPKey("tb"))

	hppOptions := mw.DefaultHPPOptions()

	secureMux := utils.ApplyMiddleware(
		router.Router(db, rdb),
		mw.Recovery, // Catch panics first
		mw.RequestID,
		mw.Cors,
		mw.ResponseTimeMiddleware,
		mw.BodySizeLimit, // Limit request body size
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
		Addr:         ":" + port,
		Handler:      secureMux,
		ReadTimeout:  15 * time.Second, // Time to read request headers + body
		WriteTimeout: 15 * time.Second, // Time to write response
		IdleTimeout:  60 * time.Second, // Keep-alive timeout
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
