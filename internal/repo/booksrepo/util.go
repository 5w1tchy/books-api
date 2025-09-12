package booksrepo

import (
	"database/sql"
	"regexp"
	"strings"
	"unicode"
)

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// replace spaces/separators with hyphen
	out := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		if unicode.IsSpace(r) || r == '_' {
			return '-'
		}
		return r
	}, s)
	out = slugRe.ReplaceAllString(out, "")
	out = strings.Trim(out, "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	if out == "" {
		out = "n-a"
	}
	return out
}

func dedupSlugs(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
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

func ensureUniqueSlug(tx *sql.Tx, table, col, base string, maxSuffix int) (string, error) {
	candidate := base
	for i := 0; i <= maxSuffix; i++ {
		if i > 0 {
			candidate = base + "-" + strconvItoa(i)
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

func strconvItoa(i int) string { return fmtInt(i) }
func fmtInt(i int) string {
	// tiny alloc-free itoa for small ints
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	n := i
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(b[pos:])
}

func getOrCreateAuthor(tx *sql.Tx, name string) (authorID string, created bool, err error) {
	// Try by normalized name (you can switch to slug if you prefer)
	if err = tx.QueryRow(`SELECT id FROM authors WHERE lower(name)=lower($1)`, name).Scan(&authorID); err == nil {
		return authorID, false, nil
	}
	if err != sql.ErrNoRows {
		return "", false, err
	}
	slugBase := slugify(name)
	slug, err2 := ensureUniqueSlug(tx, "authors", "slug", slugBase, 20)
	if err2 != nil {
		return "", false, err2
	}
	if err = tx.QueryRow(
		`INSERT INTO authors (name, slug) VALUES ($1,$2) RETURNING id`,
		name, slug,
	).Scan(&authorID); err != nil {
		return "", false, err
	}
	return authorID, true, nil
}

func resolveBookKeyCondArg(key string) (cond string, arg any) {
	// UUID | short_id | slug (basic detection)
	if isUUID(key) {
		return "b.id = $1", key
	}
	if allDigits(key) {
		return "b.short_id = $1", key
	}
	return "b.slug = $1", key
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// minimal check (you already validate in handlers)
	return true
}
func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
