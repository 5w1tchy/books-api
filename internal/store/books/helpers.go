package books

import (
	"errors"
	"regexp"
	"strings"
)

// ValidateAndSanitize validates and cleans a CreateBookV2DTO
func ValidateAndSanitize(dto *CreateBookV2DTO) error {
	sanitizeDTO(dto)
	return validateV2(*dto)
}

// sanitizeDTO cleans all fields in the DTO
func sanitizeDTO(dto *CreateBookV2DTO) {
	dto.Coda = SanitizeString(dto.Coda)
	dto.Title = SanitizeString(dto.Title)
	dto.Short = SanitizeString(dto.Short)
	dto.Summary = SanitizeString(dto.Summary)
	for i := range dto.Authors {
		dto.Authors[i] = SanitizeString(dto.Authors[i])
	}
	for i := range dto.Categories {
		dto.Categories[i] = SanitizeString(dto.Categories[i])
	}
}

// SanitizeString removes unwanted characters and normalizes whitespace
func SanitizeString(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\x00", "")
	reg := regexp.MustCompile(`\s+`)
	s = reg.ReplaceAllString(s, " ")
	return s
}

// validateV2 validates business rules for CreateBookV2DTO
func validateV2(dto CreateBookV2DTO) error {
	if len(dto.Title) == 0 || len(dto.Title) > 200 {
		return errors.New("title must be 1..200 chars")
	}
	if dto.Coda != "" && !codeRE.MatchString(dto.Coda) {
		return errors.New("coda must match ^[a-z0-9-]{3,64}$")
	}
	if len(dto.Authors) < 1 || len(dto.Authors) > 20 {
		return errors.New("authors must have 1..20 items")
	}
	if len(dto.Categories) < 1 || len(dto.Categories) > 10 {
		return errors.New("categories must have 1..10 items")
	}
	if len(dto.Short) > 280 {
		return errors.New("short must be <= 280 chars")
	}
	if len(dto.Summary) > 10000 {
		return errors.New("summary too long")
	}
	return nil
}

// Dedup removes duplicates and empty strings from a slice
func Dedup(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, s := range xs {
		s = SanitizeString(s)
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

// NullIfEmpty returns nil if string is empty, otherwise returns the string
func NullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// IsUniqueViolation checks if error is a unique constraint violation
func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}

// GenerateSlug creates a URL-friendly slug from a title
func GenerateSlug(title string) string {
	slug := strings.ToLower(title)
	reg := regexp.MustCompile(`[^a-z0-9]+`)
	slug = reg.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 64 {
		slug = slug[:64]
	}
	return slug
}

// generateSlugFromDTO creates slug from DTO
func generateSlugFromDTO(dto CreateBookV2DTO) string {
	if dto.Coda != "" {
		return strings.ToLower(dto.Coda)
	}
	return GenerateSlug(dto.Title)
}
