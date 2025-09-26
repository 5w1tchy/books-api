package password

import (
	"os"
	"strconv"
)

type Params struct {
	Memory      uint32 // kibibytes
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func loadEnvUint32(key string, def uint32) uint32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			return uint32(n)
		}
	}
	return def
}

func loadEnvUint8(key string, def uint8) uint8 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 8); err == nil {
			return uint8(n)
		}
	}
	return def
}

// Default policy ~128MB, t=3; adjust by env without code changes.
func LoadParamsFromEnv() Params {
	return Params{
		Memory:      loadEnvUint32("ARGON2_MEMORY", 131072), // 128 MiB
		Iterations:  loadEnvUint32("ARGON2_ITER", 3),
		Parallelism: loadEnvUint8("ARGON2_PAR", 1),
		SaltLength:  16,
		KeyLength:   32,
	}
}
