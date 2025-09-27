package password

import (
	"context"
	"errors"
	"strings"
)

const MinLen = 8

var (
	ErrTooShort = errors.New("weak_password.length")
)

type Warning struct {
	Score       int      `json:"score"`       // 0..4
	Message     string   `json:"message"`     // brief
	Suggestions []string `json:"suggestions"` // short hints
}

// Validate trims the password; blocks only on MinLen; returns warn-only score/info.
// Signature places error last to satisfy linters.
func Validate(ctx context.Context, pwd string, userInputs ...string) (trimmed string, warn *Warning, err error) {
	trimmed = strings.TrimSpace(pwd)

	if len(trimmed) < MinLen {
		return trimmed, nil, ErrTooShort
	}

	score, msg, sugg := simpleStrength(trimmed, userInputs...)
	if score < 3 {
		warn = &Warning{Score: score, Message: msg, Suggestions: sugg}
	}
	return trimmed, warn, nil
}

// same heuristic as used in auth handler; kept here for reuse.
func simpleStrength(pwd string, hints ...string) (int, string, []string) {
	l := len(pwd)
	var hasL, hasU, hasD, hasS bool
	for _, r := range pwd {
		switch {
		case r >= 'a' && r <= 'z':
			hasL = true
		case r >= 'A' && r <= 'Z':
			hasU = true
		case r >= '0' && r <= '9':
			hasD = true
		default:
			hasS = true
		}
	}
	classes := 0
	if hasL {
		classes++
	}
	if hasU {
		classes++
	}
	if hasD {
		classes++
	}
	if hasS {
		classes++
	}
	for _, h := range hints {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if strings.Contains(strings.ToLower(pwd), h) && l < 16 {
			if classes > 1 {
				classes--
			}
			break
		}
	}
	switch {
	case l >= 14 && classes >= 3:
		return 4, "", nil
	case l >= 12 && classes >= 3:
		return 3, "", []string{"Consider using a 3–4 word passphrase."}
	case l >= 10 && classes >= 2:
		return 2, "Short or low variety.", []string{"Add length and mix letters/numbers/symbols."}
	case l >= 8:
		return 1, "Too short or predictable.", []string{"Use at least 10–12 chars with mixed types."}
	default:
		return 0, "Very weak password.", []string{"Use 12+ chars with upper/lower, numbers, symbols."}
	}
}
