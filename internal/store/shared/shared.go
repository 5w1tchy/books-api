package shared

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// Slugify builds a stable ASCII-ish slug: [a-z0-9] with single '-' separators.
// It never drops the first character of a word and never leaves leading '-'.
func Slugify(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "n-a"
	}

	// Normalize and strip combining marks (accent folding)
	t := transform.Chain(
		norm.NFKD,
		transform.RemoveFunc(func(r rune) bool { return unicode.Is(unicode.Mn, r) }),
		norm.NFC,
	)
	normed, _, _ := transform.String(t, s)

	var b strings.Builder
	b.Grow(len(normed))
	prevDash := false

	for _, r := range normed {
		// lowercase ASCII fast path
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}

		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-' || unicode.IsSpace(r):
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		case r == '\'' || r == '’':
			// drop apostrophes entirely (no hyphen)
		default:
			// drop other punctuation/symbols
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "n-a"
	}
	for strings.Contains(out, "--") { // safety
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

// DedupSlugs normalizes via Slugify and deduplicates.
func DedupSlugs(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		slug := Slugify(s)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
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
