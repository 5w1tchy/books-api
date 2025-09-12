package shared

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	slugRe = regexp.MustCompile(`[^a-z0-9-]+`)
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Slugify lowercases, trims, replaces spaces/underscores with '-', strips non [a-z0-9-], and de-dupes '-'s.
func Slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	s = slugRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	if s == "" {
		return "n-a"
	}
	return s
}

func DedupSlugs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// EnsureUniqueSlug ensures 'base' is unique in table.col by appending -1..-maxSuffix if needed.
func EnsureUniqueSlug(tx *sql.Tx, table, col, base string, maxSuffix int) (string, error) {
	candidate := base
	for i := 0; i <= maxSuffix; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
		var exists bool
		q := `SELECT EXISTS (SELECT 1 FROM ` + table + ` WHERE ` + col + ` = $1)`
		if err := tx.QueryRow(q, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return candidate, nil
}

func IsUUID(s string) bool { return uuidRe.MatchString(s) }

// ResolveBookKeyCondArg returns a WHERE condition against alias "b" and the argument.
// UUID -> b.id=$1, numeric -> b.short_id=$1, otherwise slug -> b.slug=$1
func ResolveBookKeyCondArg(_ context.Context, key string) (cond string, arg any) {
	if IsUUID(key) {
		return "b.id = $1", key
	}
	if n, err := strconv.Atoi(key); err == nil {
		return "b.short_id = $1", n
	}
	return "b.slug = $1", key
}
