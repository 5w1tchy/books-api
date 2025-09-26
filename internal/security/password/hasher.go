package password

import (
	"github.com/alexedwards/argon2id"
)

var policy = LoadParamsFromEnv()

// Hash returns a PHC string like `$argon2id$v=19$m=131072,t=3,p=1$...`
func Hash(plain string) (string, error) {
	p := argon2id.Params{
		Memory:      policy.Memory,
		Iterations:  policy.Iterations,
		Parallelism: policy.Parallelism,
		SaltLength:  policy.SaltLength,
		KeyLength:   policy.KeyLength,
	}
	return argon2id.CreateHash(plain, &p)
}

// Verify checks password vs PHC hash and also indicates if a rehash is recommended.
func Verify(plain, phc string) (ok bool, needsRehash bool, err error) {
	ok, err = argon2id.ComparePasswordAndHash(plain, phc)
	if err != nil || !ok {
		return ok, false, err
	}
	return ok, NeedsRehash(phc), nil
}

func NeedsRehash(phc string) bool {
	stored, _, _, err := argon2id.DecodeHash(phc)
	if err != nil {
		// Can't parse: treat as needs rehash.
		return true
	}
	return stored.Memory < policy.Memory ||
		stored.Iterations < policy.Iterations ||
		stored.Parallelism < policy.Parallelism ||
		stored.SaltLength < policy.SaltLength ||
		stored.KeyLength < policy.KeyLength
}
