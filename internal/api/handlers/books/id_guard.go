package books

import (
	"regexp"
	"strings"
)

// Accepts any RFC4122 variant (v1–v5). Lowercased for simplicity.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[ab89][0-9a-f]{3}-[0-9a-f]{12}$`)

func isUUID(s string) bool {
	return uuidRe.MatchString(strings.ToLower(s))
}
