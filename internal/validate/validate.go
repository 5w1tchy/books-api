package validate

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid = errors.New("invalid")
	slugRe     = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// RequireBounded trims and ensures length bounds.
func RequireBounded(name, s string, min, max int) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) < min || utf8.RuneCountInString(s) > max {
		return "", errors.New(name + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max) + " characters")
	}
	return s, nil
}

// ParseCategoriesCSV: "a,b,c" -> []{"a","b","c"} (lowercased/deduped).
func ParseCategoriesCSV(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, p := range strings.Split(csv, ",") {
		s := strings.ToLower(strings.TrimSpace(p))
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

// ValidateCategorySlugs checks count and slug shape.
func ValidateCategorySlugs(slugs []string, maxCount int) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(slugs))
	for _, raw := range slugs {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" || !slugRe.MatchString(s) || utf8.RuneCountInString(s) > 50 {
			return nil, errors.New("invalid category slug: " + raw)
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) > maxCount {
		return nil, errors.New("too many categories")
	}
	return out, nil
}

func ParseMatch(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "all" {
		return "all"
	}
	return "any"
}

// ClampLimitOffset parses and clamps paging.
func ClampLimitOffset(limitRaw, offsetRaw string, def, max int) (int, int) {
	limit := def
	if v, err := strconv.Atoi(strings.TrimSpace(limitRaw)); err == nil && v >= 1 && v <= max {
		limit = v
	}
	offset := 0
	if v, err := strconv.Atoi(strings.TrimSpace(offsetRaw)); err == nil && v >= 0 {
		offset = v
	}
	return limit, offset
}

// ParseMinSim applies defaults for short/long queries and optional override.
func ParseMinSim(query, override string) float64 {
	if strings.TrimSpace(query) == "" {
		return 0 // unused when q==""
	}
	def := 0.20
	if utf8.RuneCountInString(query) <= 3 {
		def = 0.10
	}
	if override == "" {
		return def
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(override), 64); err == nil && f >= 0 && f <= 1 {
		return f
	}
	return def
}

// ParseFields returns a set of allowed fields (case/space-insensitive).
func ParseFields(raw string, allowed []string) map[string]struct{} {
	set := map[string]struct{}{}
	if strings.TrimSpace(raw) == "" {
		return set
	}
	allow := map[string]struct{}{}
	for _, a := range allowed {
		allow[a] = struct{}{}
	}
	for _, f := range strings.Split(raw, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if _, ok := allow[f]; ok {
			set[f] = struct{}{}
		}
	}
	return set
}
