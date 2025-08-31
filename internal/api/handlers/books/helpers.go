package books

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func resolveBookKey(key string) (cond string, arg any) {
	if isDigits(key) {
		n, _ := strconv.ParseInt(key, 10, 64)
		return "b.short_id = $1", n
	}
	if looksLikeUUID(key) {
		return "b.id = $1", key
	}
	return "b.slug = $1", key
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func looksLikeUUID(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

func dedupSlugs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
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

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, _ := transform.String(t, s)
	s = normalized
	reNon := regexp.MustCompile(`[^a-z0-9]+`)
	reDash := regexp.MustCompile(`-+`)
	s = reNon.ReplaceAllString(s, "-")
	s = reDash.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "item"
	}
	return s
}

func ensureUniqueSlug(tx *sql.Tx, table, col, base string, maxTries int) (string, error) {
	slug := base
	for i := 1; i <= maxTries; i++ {
		var exists bool
		q := `SELECT EXISTS (SELECT 1 FROM ` + table + ` WHERE ` + col + ` = $1)`
		if err := tx.QueryRow(q, slug).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = base + "-" + strconv.Itoa(i+1)
	}
	return "", fmt.Errorf("could not create unique slug for %q", base)
}

func getOrCreateAuthor(tx *sql.Tx, name string) (id string, slug string, err error) {
	err = tx.QueryRow(`SELECT id, slug FROM authors WHERE name = $1`, name).Scan(&id, &slug)
	if err == nil {
		return id, slug, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}

	base := slugify(name)
	slug, err = ensureUniqueSlug(tx, "authors", "slug", base, 10)
	if err != nil {
		return "", "", err
	}
	if err = tx.QueryRow(`INSERT INTO authors (name, slug) VALUES ($1, $2) RETURNING id`, name, slug).Scan(&id); err != nil {
		return "", "", err
	}
	return id, slug, nil
}
