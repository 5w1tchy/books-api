package admin

import (
	"database/sql"

	"github.com/redis/go-redis/v9"
)

type Handler struct {
	DB  *sql.DB
	RDB *redis.Client
	Sto Store
}

func NewHandler(db *sql.DB, rdb *redis.Client, store Store) *Handler {
	return &Handler{
		DB:  db,
		RDB: rdb,
		Sto: store,
	}
}
