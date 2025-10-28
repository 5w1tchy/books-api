package auth

import "strings"

// simpleStrength gives a coarse 0..4 score + short warning/suggestions (warn-only use).
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

	// small penalty if password contains a hint (like email/user)
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

	// scoring heuristic
	switch {
	case l >= 14 && classes >= 3:
		return 4, "", nil
	case l >= 12 && classes >= 3:
		return 3, "", []string{"Consider using 3–4 word passphrase for even stronger security."}
	case l >= 10 && classes >= 2:
		return 2, "Short or low variety.", []string{"Add more length and mix of letters, numbers, symbols."}
	case l >= 8:
		return 1, "Too short or predictable.", []string{"Use at least 10–12 chars with mixed types."}
	default:
		return 0, "Very weak password.", []string{"Use 12+ chars with upper/lower, numbers, symbols."}
	}
}
